package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"time"

	"ceitg/internal/icon"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	colBG      = color.RGBA{0x12, 0x14, 0x1c, 0xff}
	colField   = color.RGBA{0x1a, 0x1e, 0x2a, 0xff}
	colZone    = color.RGBA{0x1b, 0x33, 0x2f, 0xff}
	colZoneHi  = color.RGBA{0x22, 0x4a, 0x41, 0xff}
	colLine    = color.RGBA{0x2c, 0x32, 0x44, 0xff}
	colPanel   = color.RGBA{0x20, 0x24, 0x33, 0xff}
	colPanelHi = color.RGBA{0x2a, 0x30, 0x44, 0xff}
	colText    = color.RGBA{0xe8, 0xea, 0xf0, 0xff}
	colDim     = color.RGBA{0x8a, 0x90, 0xa3, 0xff}
	colRunner  = color.RGBA{0x4c, 0xc9, 0xf0, 0xff}
	colCatcher = color.RGBA{0xff, 0x5d, 0x73, 0xff}
	colNeutral = color.RGBA{0xb8, 0xbc, 0xcc, 0xff}
	colEnergy  = color.RGBA{0xff, 0xd1, 0x66, 0xff}
	colStick   = color.RGBA{0xc8, 0xa2, 0x6b, 0xff}
	colGold    = color.RGBA{0xff, 0xd1, 0x66, 0xff}
	colInk     = color.RGBA{0x10, 0x12, 0x18, 0xff}
)

func alpha(c color.RGBA, a float64) color.RGBA {
	f := math.Max(0, math.Min(1, a))
	return color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), uint8(255 * f)}
}

func teamColor(t uint8) color.RGBA {
	switch t {
	case TeamRunner:
		return colRunner
	case TeamCatcher:
		return colCatcher
	}
	return colNeutral
}

func box(dst *ebiten.Image, r rect, c color.Color) {
	vector.FillRect(dst, r.x, r.y, r.w, r.h, c, false)
}

func button(dst *ebiten.Image, r rect, label string, enabled bool, accent color.RGBA) {
	mx, my := ebiten.CursorPosition()
	bg := colPanel
	if enabled {
		bg = accent
		if r.has(mx, my) {
			bg = alpha(accent, 0.85)
		}
	}
	box(dst, r, bg)
	tc := color.Color(colInk)
	if !enabled {
		tc = colDim
	}
	drawText(dst, label, float64(r.x+r.w/2), float64(r.y+r.h/2), 20, true, tc, text.AlignCenter)
}

func inputBox(dst *ebiten.Image, r rect, val, placeholder string, focused bool, blink bool) {
	box(dst, r, colPanel)
	if focused {
		vector.StrokeRect(dst, r.x, r.y, r.w, r.h, 2, colRunner, false)
	}
	tx, ty := float64(r.x+16), float64(r.y+r.h/2)
	if val == "" && !focused {
		drawText(dst, placeholder, tx, ty, 18, false, colDim, text.AlignStart)
		return
	}
	s := val
	if focused && blink {
		s += "|"
	}
	drawText(dst, s, tx, ty, 18, false, colText, text.AlignStart)
}

func (g *Game) Draw(dst *ebiten.Image) {
	if !g.dirty {
		return
	}
	g.dirty = false
	dst.Fill(colBG)
	if g.scene == sceneMenu {
		g.drawMenu(dst)
		return
	}
	if g.state.Phase == PhaseLobby {
		g.drawLobby(dst)
	} else {
		g.drawMatch(dst)
	}
	if time.Since(g.escAt) < 1500*time.Millisecond {
		drawText(dst, "Press Esc again to leave", W/2, H-24, 16, true, colEnergy, text.AlignCenter)
	}
}

// ---------- menu ----------

func (g *Game) drawMenu(dst *ebiten.Image) {
	blink := (g.frame/30)%2 == 0
	drawText(dst, "CeitG", W/2, 112, 64, true, colText, text.AlignCenter)
	drawText(dst, "cross the field 3 times  ·  don't get tagged", W/2, 168, 18, false, colDim, text.AlignCenter)

	drawText(dst, "YOUR NAME", float64(rName.x), float64(rName.y-14), 13, true, colDim, text.AlignStart)
	inputBox(dst, rName, g.name, "type your name…", g.focus == 0, blink)
	button(dst, rHost, "Host a game", true, colRunner)

	drawText(dst, "GAMES ON THIS NETWORK", float64(rHost.x), 404, 13, true, colDim, text.AlignStart)
	if len(g.found) == 0 {
		drawText(dst, "searching…", float64(rHost.x), 450, 17, false, colDim, text.AlignStart)
	}
	mx, my := ebiten.CursorPosition()
	for i := 0; i < len(g.found) && i < maxFound; i++ {
		r := foundRect(i)
		bg := colPanel
		if r.has(mx, my) {
			bg = colPanelHi
		}
		box(dst, r, bg)
		f := g.found[i]
		drawText(dst, f.Name+"'s game", float64(r.x+16), float64(r.y+r.h/2), 18, true, colText, text.AlignStart)
		drawText(dst, fmt.Sprintf("%d players  ·  join →", f.Players), float64(r.x+r.w-16), float64(r.y+r.h/2), 15, false, colRunner, text.AlignEnd)
	}

	drawText(dst, "OR JOIN BY IP", float64(rIP.x), float64(rIP.y-14), 13, true, colDim, text.AlignStart)
	inputBox(dst, rIP, g.ip, "192.168.1.20", g.focus == 1, blink)
	button(dst, rJoinIP, "Join", true, colRunner)

	if g.status != "" {
		drawText(dst, g.status, W/2, 700, 16, true, colCatcher, text.AlignCenter)
	}

	// controls card
	x := 905.0
	drawText(dst, "CONTROLS", x, 244, 13, true, colDim, text.AlignStart)
	lines := [][2]string{{"WASD / Arrows", "move"}, {"Shift", "run (uses energy)"}, {"Space / J / Click", "hit (catchers)"}, {"F11", "fullscreen"}, {"Esc ×2", "leave game"}}
	for i, l := range lines {
		y := 274 + float64(i)*30
		drawText(dst, l[0], x, y, 15, true, colText, text.AlignStart)
		drawText(dst, l[1], x+150, y, 15, false, colDim, text.AlignStart)
	}
	drawText(dst, "RULES", x, 444, 13, true, colDim, text.AlignStart)
	rules := []string{"Teams split in half — catchers", "get the extra player.", "Runners: reach the other side", "3 times. Side zones are safe.", "Tagged runners become catchers."}
	for i, l := range rules {
		drawText(dst, l, x, 474+float64(i)*24, 15, false, colText, text.AlignStart)
	}
}

// ---------- lobby ----------

func (g *Game) drawLobby(dst *ebiten.Image) {
	drawText(dst, "Lobby", W/2, 70, 40, true, colText, text.AlignCenter)
	if g.server != nil {
		ips := g.hostIPs
		if ips == "" {
			ips = "no network found"
		}
		drawText(dst, "You are hosting. Friends join from the menu, or by typing:", W/2, 120, 16, false, colDim, text.AlignCenter)
		drawText(dst, ips, W/2, 150, 24, true, colEnergy, text.AlignCenter)
	} else {
		drawText(dst, "Connected. Waiting for the host to start…", W/2, 130, 18, false, colDim, text.AlignCenter)
	}

	ps := append([]PlayerState(nil), g.state.Players...)
	sort.Slice(ps, func(i, j int) bool { return ps[i].ID < ps[j].ID })
	cols := 10
	for i, p := range ps {
		cx := float32(W/2 - (min(len(ps), cols)-1)*55 + (i%cols)*110)
		cy := float32(250 + (i/cols)*90)
		vector.FillCircle(dst, cx, cy, Radius*1.4, colNeutral, true)
		if p.ID == g.myID {
			vector.StrokeCircle(dst, cx, cy, Radius*1.4+4, 3, colText, true)
		}
		op := &text.DrawOptions{}
		op.GeoM.Translate(float64(cx), float64(cy))
		op.ColorScale.ScaleWithColor(colInk)
		op.PrimaryAlign, op.SecondaryAlign = text.AlignCenter, text.AlignCenter
		text.Draw(dst, p.Name, nameFace(p.Name, Radius*1.4), op)
		if p.Host {
			drawText(dst, "host", float64(cx), float64(cy+Radius*1.4+14), 12, false, colEnergy, text.AlignCenter)
		}
	}

	n := len(ps)
	split := fmt.Sprintf("%d players  →  %d catchers  vs  %d runners", n, (n+1)/2, n/2)
	drawText(dst, split, W/2, 560, 18, true, colText, text.AlignCenter)
	if g.server != nil {
		label := "Start (Enter)"
		if n < 2 {
			label = "Need 2+ players"
		}
		button(dst, rStart, label, n >= 2, colCatcher)
	}
}

// ---------- match ----------

func (g *Game) me() *PlayerState {
	for i := range g.state.Players {
		if g.state.Players[i].ID == g.myID {
			return &g.state.Players[i]
		}
	}
	return nil
}

func (g *Game) drawMatch(dst *ebiten.Image) {
	me := g.me()

	// field
	box(dst, rect{0, ArenaTop, W, H - ArenaTop}, colField)
	lz, rz := colZone, colZone
	if me != nil && me.Team == TeamRunner && !me.Esc && g.state.Phase == PhasePlaying {
		pulse := 0.5 + 0.5*math.Sin(float64(g.frame)/12)
		hi := color.RGBA{
			uint8(float64(colZone.R) + (float64(colZoneHi.R)-float64(colZone.R))*pulse),
			uint8(float64(colZone.G) + (float64(colZoneHi.G)-float64(colZone.G))*pulse),
			uint8(float64(colZone.B) + (float64(colZoneHi.B)-float64(colZone.B))*pulse), 0xff}
		if me.C%2 == 0 {
			rz = hi
		} else {
			lz = hi
		}
	}
	box(dst, rect{0, ArenaTop, ZoneW, H - ArenaTop}, lz)
	box(dst, rect{W - ZoneW, ArenaTop, ZoneW, H - ArenaTop}, rz)
	for y := float32(ArenaTop + 8); y < H; y += 24 {
		vector.StrokeLine(dst, W/2, y, W/2, y+12, 2, colLine, false)
	}
	drawText(dst, "SAFE", ZoneW/2, H-20, 14, true, alpha(colRunner, 0.35), text.AlignCenter)
	drawText(dst, "SAFE", W-ZoneW/2, H-20, 14, true, alpha(colRunner, 0.35), text.AlignCenter)

	// run trails (under everyone)
	for _, p := range g.state.Players {
		v := g.view[p.ID]
		if v == nil || p.Team == TeamNone {
			continue
		}
		for i, t := range v.trail {
			f := float64(i+1) / float64(len(v.trail)+1)
			vector.FillCircle(dst, float32(t[0]), float32(t[1]), float32(Radius*(0.45+0.4*f)), alpha(teamColor(p.Team), 0.22*f), true)
		}
	}

	for _, b := range g.bursts {
		f := b.t / 0.45
		vector.StrokeCircle(dst, float32(b.x), float32(b.y), float32(Radius+40*f), 3, alpha(colCatcher, 1-f), true)
	}

	// players: draw me last so I'm always on top
	order := make([]PlayerState, 0, len(g.state.Players))
	for _, p := range g.state.Players {
		if p.Team != TeamNone && p.ID != g.myID {
			order = append(order, p)
		}
	}
	if me != nil && me.Team != TeamNone {
		order = append(order, *me)
	}
	for _, p := range order {
		g.drawPlayer(dst, p, p.ID == g.myID)
	}

	g.drawHUD(dst, me)

	// event feed
	if g.lastMsg != "" && g.state.Phase == PhasePlaying {
		if age := time.Since(g.msgAt).Seconds(); age < 3 {
			a := math.Min(1, 3-age)
			w, _ := text.Measure(g.lastMsg, face(17, true), 0)
			box(dst, rect{float32(W/2 - w/2 - 16), ArenaTop + 14, float32(w + 32), 34}, alpha(colPanel, 0.9*a))
			drawText(dst, g.lastMsg, W/2, ArenaTop+31, 17, true, alpha(colText, a), text.AlignCenter)
		}
	}

	switch g.state.Phase {
	case PhaseCountdown:
		n := int(math.Ceil(float64(g.state.Time)))
		box(dst, rect{0, H/2 - 90, W, 230}, alpha(colBG, 0.8))
		drawText(dst, fmt.Sprint(n), W/2, H/2, 120, true, alpha(colText, 0.9), text.AlignCenter)
		if me != nil {
			role := map[uint8]string{TeamRunner: "You are a RUNNER — cross 3 times!", TeamCatcher: "You are a CATCHER — tag the runners!"}[me.Team]
			drawText(dst, role, W/2, H/2+90, 24, true, teamColor(me.Team), text.AlignCenter)
		}
	case PhaseResults:
		box(dst, rect{0, H/2 - 80, W, 160}, alpha(colBG, 0.88))
		c := colCatcher
		if len(g.state.Msg) > 0 && g.state.Msg[0] == 'R' {
			c = colRunner
		}
		drawText(dst, g.state.Msg, W/2, H/2-14, 44, true, c, text.AlignCenter)
		drawText(dst, fmt.Sprintf("back to lobby in %d…", int(math.Ceil(float64(g.state.Time)))), W/2, H/2+42, 17, false, colDim, text.AlignCenter)
	}
}

func (g *Game) drawPlayer(dst *ebiten.Image, p PlayerState, mine bool) {
	v := g.view[p.ID]
	if v == nil {
		return
	}
	x, y := float32(v.x), float32(v.y)
	c := teamColor(p.Team)
	if p.Esc {
		c = alpha(c, 0.55)
	}

	if p.Team == TeamCatcher {
		base := math.Atan2(float64(p.FY), float64(p.FX))
		a := base + 0.9
		if p.Swing > 0 {
			a = base + 1.4 - 2.8*float64(p.Swing)
		}
		ca, sa := float32(math.Cos(a)), float32(math.Sin(a))
		vector.StrokeLine(dst, x+ca*(Radius-4), y+sa*(Radius-4), x+ca*(Radius+StickLen), y+sa*(Radius+StickLen), 6, colStick, true)
		if p.Swing > 0 {
			sw := alpha(colText, 0.25*(1-float64(p.Swing)))
			vector.StrokeCircle(dst, x, y, Radius+StickLen-4, 2, sw, true)
		}
	}

	vector.FillCircle(dst, x, y, Radius, c, true)
	if mine {
		vector.StrokeCircle(dst, x, y, Radius+3, 3, colText, true)
	}
	if p.Esc {
		vector.StrokeCircle(dst, x, y, Radius+3, 2, colGold, true)
	}
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(colInk)
	op.PrimaryAlign, op.SecondaryAlign = text.AlignCenter, text.AlignCenter
	text.Draw(dst, p.Name, nameFace(p.Name, Radius), op)

	if p.Team == TeamRunner && !p.Esc {
		for i := 0; i < CrossToWin; i++ { // crossing pips under runners
			pc := alpha(colText, 0.25)
			if i < int(p.C) {
				pc = colGold
			}
			vector.FillCircle(dst, x-10+float32(i)*10, y+Radius+8, 3, pc, true)
		}
	}
}

func (g *Game) drawHUD(dst *ebiten.Image, me *PlayerState) {
	box(dst, rect{0, 0, W, ArenaTop}, colBG)

	runners, catchers := 0, 0
	for _, p := range g.state.Players {
		switch p.Team {
		case TeamRunner:
			if !p.Esc {
				runners++
			}
		case TeamCatcher:
			catchers++
		}
	}
	t := int(math.Ceil(float64(g.clock)))
	tc := colText
	if t <= 15 && g.state.Phase == PhasePlaying {
		tc = colCatcher
	}
	cy := float64(ArenaTop / 2)
	drawText(dst, fmt.Sprintf("%d:%02d", t/60, t%60), W/2, cy, 26, true, tc, text.AlignCenter)
	vector.FillCircle(dst, W/2-100, float32(cy), 7, colRunner, true)
	drawText(dst, fmt.Sprint(runners), W/2-86, cy, 20, true, colText, text.AlignStart)
	vector.FillCircle(dst, W/2+100, float32(cy), 7, colCatcher, true)
	drawText(dst, fmt.Sprint(catchers), W/2+86, cy, 20, true, colText, text.AlignEnd)

	if me == nil || me.Team == TeamNone {
		drawText(dst, "Spectating — you'll play next round", 20, cy, 17, true, colDim, text.AlignStart)
		return
	}
	switch {
	case me.Team == TeamRunner && me.Esc:
		drawText(dst, "ESCAPED! ★", 20, cy, 18, true, colGold, text.AlignStart)
	case me.Team == TeamRunner:
		dir := "→ go RIGHT"
		if me.C%2 == 1 {
			dir = "← go LEFT"
		}
		drawText(dst, fmt.Sprintf("RUNNER  %d/%d  %s", me.C, CrossToWin, dir), 20, cy, 18, true, colRunner, text.AlignStart)
	default:
		drawText(dst, "CATCHER  — tag runners with Space", 20, cy, 18, true, colCatcher, text.AlignStart)
	}

	// energy bar
	bx, bw := float32(W-300), float32(180)
	drawText(dst, "RUN", float64(bx-10), cy, 13, true, colDim, text.AlignEnd)
	box(dst, rect{bx, float32(cy) - 6, bw, 12}, colPanel)
	ec := colEnergy
	if float64(me.E) < EnergyMinGo && !me.Run {
		ec = alpha(colEnergy, 0.45)
	}
	box(dst, rect{bx, float32(cy) - 6, bw * float32(me.E) / EnergyMax, 12}, ec)

	if me.Team == TeamCatcher {
		hx, hw := float32(W-70), float32(50)
		drawText(dst, "HIT", float64(hx-10), cy, 13, true, colDim, text.AlignEnd)
		box(dst, rect{hx, float32(cy) - 6, hw, 12}, colPanel)
		box(dst, rect{hx, float32(cy) - 6, hw * (1 - me.CD), 12}, colStick)
	}
}

func appIcons() []image.Image {
	var out []image.Image
	for _, n := range []int{16, 32, 48, 64, 128} {
		out = append(out, icon.Draw(n))
	}
	return out
}
