package main

import (
	"bytes"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

type scene int

const (
	sceneMenu scene = iota
	sceneGame
)

type rect struct{ x, y, w, h float32 }

func (r rect) has(x, y int) bool {
	fx, fy := float32(x), float32(y)
	return fx >= r.x && fx <= r.x+r.w && fy >= r.y && fy <= r.y+r.h
}

type viewP struct {
	x, y  float64
	trail [][2]float64
	team  uint8
}

type burst struct {
	x, y, t float64
}

type Game struct {
	scene  scene
	name   string
	ip     string
	focus  int // 0 = name field, 1 = ip field
	status string

	finder  *Finder
	found   []FoundGame
	server  *Server
	stopAnn chan struct{}
	client  *Client
	hostIPs string

	state     ServerMsg
	myID      int
	view      map[int]*viewP
	bursts    []burst
	frame     int
	lastMsg   string
	msgAt     time.Time
	escAt     time.Time
	backspace int
	dirty     bool    // something changed since the last Draw
	clock     float32 // round clock shown in the HUD (frozen on results)
}

// ---------- fonts ----------

var (
	fontReg, fontBold *text.GoTextFaceSource
	faceCache         = map[[2]float64]*text.GoTextFace{}
	nameFaceCache     = map[nameKey]*text.GoTextFace{}
)

func face(size float64, bold bool) *text.GoTextFace {
	k := [2]float64{size, 0}
	src := fontReg
	if bold {
		k[1], src = 1, fontBold
	}
	if f, ok := faceCache[k]; ok {
		return f
	}
	f := &text.GoTextFace{Source: src, Size: size}
	faceCache[k] = f
	return f
}

type nameKey struct {
	name string
	r    float32
}

// nameFace picks the biggest font size that lets the name fit inside a circle of radius r.
func nameFace(name string, r float32) *text.GoTextFace {
	k := nameKey{name, r}
	if f, ok := nameFaceCache[k]; ok {
		return f
	}
	maxW := float64(r*2 - 8)
	size := float64(r) * 0.7
	for ; size > 6; size -= 0.5 {
		if w, _ := text.Measure(name, face(size, true), 0); w <= maxW {
			break
		}
	}
	f := face(size, true)
	nameFaceCache[k] = f
	return f
}

func drawText(dst *ebiten.Image, s string, x, y, size float64, bold bool, c color.Color, align text.Align) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	op.PrimaryAlign = align
	op.SecondaryAlign = text.AlignCenter
	text.Draw(dst, s, face(size, bold), op)
}

// ---------- name persistence ----------

func configPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "CeitG", "name.txt")
}

func loadName() string {
	if p := configPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}

func saveName(n string) {
	if p := configPath(); p != "" {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(n), 0o644)
	}
}

// ---------- menu layout (shared by Update and Draw) ----------

var (
	rName   = rect{440, 236, 400, 50}
	rHost   = rect{440, 304, 400, 54}
	rIP     = rect{440, 630, 290, 46}
	rJoinIP = rect{740, 630, 100, 46}
	rStart  = rect{490, 600, 300, 56}
)

func foundRect(i int) rect { return rect{440, 428 + float32(i)*52, 400, 44} }

const maxFound = 3

// ---------- update ----------

func (g *Game) Update() error {
	g.frame++
	// Menus/lobby redraw at ~20 fps (or instantly on input); the match at the tick rate.
	if g.scene == sceneGame && g.state.Phase != PhaseLobby || g.frame%3 == 0 ||
		len(inpututil.AppendJustPressedKeys(nil)) > 0 || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.dirty = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	if g.scene == sceneMenu {
		g.updateMenu()
	} else {
		g.updateGame()
	}
	return nil
}

func (g *Game) updateMenu() {
	g.found = g.finder.List()
	mx, my := ebiten.CursorPosition()
	click := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)

	if click {
		switch {
		case rName.has(mx, my):
			g.focus = 0
		case rIP.has(mx, my):
			g.focus = 1
		case rHost.has(mx, my):
			g.host()
		case rJoinIP.has(mx, my):
			g.join(strings.TrimSpace(g.ip))
		}
		for i := 0; i < len(g.found) && i < maxFound; i++ {
			if foundRect(i).has(mx, my) {
				g.join(g.found[i].Addr)
			}
		}
	}
	if g.scene != sceneMenu {
		return
	}

	field := &g.name
	allow := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' }
	limit := MaxNameLen
	if g.focus == 1 {
		field = &g.ip
		allow = func(r rune) bool { return r == '.' || (r >= '0' && r <= '9') }
		limit = 15
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if allow(r) && len([]rune(*field)) < limit {
			*field += string(r)
		}
	}
	if ebiten.IsKeyPressed(ebiten.KeyBackspace) {
		if g.backspace == 0 || (g.backspace > 25 && g.backspace%3 == 0) {
			if rs := []rune(*field); len(rs) > 0 {
				*field = string(rs[:len(rs)-1])
			}
		}
		g.backspace++
	} else {
		g.backspace = 0
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		g.focus = 1 - g.focus
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		if g.focus == 1 {
			g.join(strings.TrimSpace(g.ip))
		} else {
			g.host()
		}
	}
}

func (g *Game) playerName() string {
	n := strings.TrimSpace(g.name)
	if n == "" {
		return "Player"
	}
	return n
}

func (g *Game) host() {
	if strings.TrimSpace(g.name) == "" {
		g.status, g.focus = "Type your name first", 0
		return
	}
	s, err := StartServer()
	if err != nil {
		g.status = "Can't host: port busy (is another CeitG host open on this PC?)"
		return
	}
	c, err := Connect("127.0.0.1", g.playerName())
	if err != nil {
		s.Close()
		g.status = "Can't host: " + err.Error()
		return
	}
	g.server, g.client = s, c
	g.stopAnn = make(chan struct{})
	go Announce(g.stopAnn, g.playerName(), s.PlayerCount)
	g.hostIPs = strings.Join(LocalIPs(), "   or   ")
	g.enterGame()
}

func (g *Game) join(addr string) {
	if strings.TrimSpace(g.name) == "" {
		g.status, g.focus = "Type your name first", 0
		return
	}
	if addr == "" {
		g.status, g.focus = "Type the host's IP address", 1
		return
	}
	c, err := Connect(addr, g.playerName())
	if err != nil {
		g.status = "Couldn't connect to " + addr
		return
	}
	g.client = c
	g.enterGame()
}

func (g *Game) enterGame() {
	saveName(g.playerName())
	g.scene, g.status = sceneGame, ""
	g.view = map[int]*viewP{}
	g.bursts = nil
	g.state = ServerMsg{}
	g.lastMsg = ""
}

func (g *Game) leave(reason string) {
	if g.client != nil {
		g.client.Close()
		g.client = nil
	}
	if g.server != nil {
		close(g.stopAnn)
		g.server.Close()
		g.server = nil
	}
	g.scene, g.status = sceneMenu, reason
}

func (g *Game) updateGame() {
	st, id, fresh, err := g.client.Snapshot()
	if err != nil {
		if g.server != nil {
			g.leave("Game closed")
		} else {
			g.leave("Disconnected: the host closed the game or the network dropped")
		}
		return
	}
	if fresh {
		if st.Phase == PhasePlaying {
			g.clock = st.Time
		} else if st.Phase != PhaseResults {
			g.clock = RoundTime
		}
		g.state, g.myID = st, id
		if st.Msg != g.lastMsg {
			g.lastMsg, g.msgAt = st.Msg, time.Now()
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if time.Since(g.escAt) < 1500*time.Millisecond {
			g.leave("")
			return
		}
		g.escAt = time.Now()
	}

	if g.state.Phase == PhaseLobby && g.server != nil {
		mx, my := ebiten.CursorPosition()
		if (inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && rStart.has(mx, my)) ||
			inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			g.server.StartRound()
		}
	}

	var k uint8
	if ebiten.IsFocused() {
		key := ebiten.IsKeyPressed
		if key(ebiten.KeyW) || key(ebiten.KeyArrowUp) {
			k |= InUp
		}
		if key(ebiten.KeyS) || key(ebiten.KeyArrowDown) {
			k |= InDown
		}
		if key(ebiten.KeyA) || key(ebiten.KeyArrowLeft) {
			k |= InLeft
		}
		if key(ebiten.KeyD) || key(ebiten.KeyArrowRight) {
			k |= InRight
		}
		if key(ebiten.KeyShift) {
			k |= InRun
		}
		if key(ebiten.KeySpace) || key(ebiten.KeyJ) || ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			k |= InHit
		}
	}
	g.client.SendInput(k)

	// smooth positions, run trails and "caught" bursts (client-side only)
	seen := map[int]bool{}
	for _, p := range g.state.Players {
		seen[p.ID] = true
		v := g.view[p.ID]
		tx, ty := float64(p.X), float64(p.Y)
		if v == nil {
			v = &viewP{x: tx, y: ty, team: p.Team}
			g.view[p.ID] = v
		}
		if dx, dy := tx-v.x, ty-v.y; dx*dx+dy*dy > 150*150 {
			v.x, v.y, v.trail = tx, ty, nil
		} else {
			v.x += dx * 0.35
			v.y += dy * 0.35
		}
		if v.team == TeamRunner && p.Team == TeamCatcher && g.state.Phase == PhasePlaying {
			g.bursts = append(g.bursts, burst{x: v.x, y: v.y})
		}
		v.team = p.Team
		if g.frame%2 == 0 {
			if p.Run {
				v.trail = append(v.trail, [2]float64{v.x, v.y})
				if len(v.trail) > 5 {
					v.trail = v.trail[1:]
				}
			} else if len(v.trail) > 0 {
				v.trail = v.trail[1:]
			}
		}
	}
	for id := range g.view {
		if !seen[id] {
			delete(g.view, id)
		}
	}
	live := g.bursts[:0]
	for _, b := range g.bursts {
		b.t += 1.0 / 60
		if b.t < 0.45 {
			live = append(live, b)
		}
	}
	g.bursts = live
}

func (g *Game) Layout(_, _ int) (int, int) { return W, H }

func main() {
	var err error
	if fontReg, err = text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF)); err != nil {
		log.Fatal(err)
	}
	if fontBold, err = text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF)); err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowTitle("CeitG")
	ebiten.SetWindowSize(1152, 648)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowIcon(appIcons())
	ebiten.SetScreenClearedEveryFrame(false) // lets Draw skip frames where nothing changed
	g := &Game{name: loadName(), finder: NewFinder()}
	if g.name != "" {
		g.focus = 1
	}
	defer g.finder.Close()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
	g.leave("")
}
