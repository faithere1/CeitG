package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

type sPlayer struct {
	PlayerState
	vx, vy    float64
	x, y      float64
	energy    float64
	swingT    float64
	dashT     float64
	cooldown  float64
	input     uint8
	hitQueued bool
	goalRight bool
	hitDone   map[int]bool
	out       chan []byte
	conn      net.Conn
}

type Server struct {
	mu      sync.Mutex
	ln      net.Listener
	players map[int]*sPlayer
	nextID  int
	hostID  int
	phase   uint8
	timer   float64
	msg     string
	closed  chan struct{}
}

func StartServer() (*Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", GamePort))
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, players: map[int]*sPlayer{}, nextID: 1, closed: make(chan struct{})}
	go s.acceptLoop()
	go s.tickLoop()
	return s, nil
}

func (s *Server) Close() {
	select {
	case <-s.closed:
		return
	default:
	}
	close(s.closed)
	s.ln.Close()
	s.mu.Lock()
	for _, p := range s.players {
		p.conn.Close()
	}
	s.mu.Unlock()
}

func (s *Server) PlayerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.players)
}

func (s *Server) acceptLoop() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	var hello ClientMsg
	if json.Unmarshal(line, &hello) != nil || hello.T != "hello" {
		return
	}
	c.SetReadDeadline(time.Time{})

	s.mu.Lock()
	p := &sPlayer{conn: c, out: make(chan []byte, 16)}
	p.ID = s.nextID
	s.nextID++
	p.Name = cleanName(hello.Name, s.players)
	p.energy = EnergyMax
	p.FX = 1
	p.x, p.y = W/2, H/2
	if s.hostID == 0 {
		s.hostID = p.ID
	}
	p.Host = p.ID == s.hostID
	s.players[p.ID] = p
	if s.phase != PhaseLobby {
		s.msg = p.Name + " joined (spectating until next round)"
	}
	s.mu.Unlock()

	welcome, _ := json.Marshal(ServerMsg{T: "welcome", ID: p.ID})
	p.out <- append(welcome, '\n')

	go func() {
		w := bufio.NewWriter(c)
		for b := range p.out {
			w.Write(b)
			// flush once the queue is drained to batch writes
			if len(p.out) == 0 {
				if w.Flush() != nil {
					return
				}
			}
		}
	}()

	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		var m ClientMsg
		if json.Unmarshal(line, &m) != nil || m.T != "in" {
			continue
		}
		s.mu.Lock()
		if m.K&InHit != 0 && p.input&InHit == 0 && s.phase == PhasePlaying {
			p.hitQueued = true
		}
		p.input = m.K
		s.mu.Unlock()
	}

	s.mu.Lock()
	delete(s.players, p.ID)
	close(p.out)
	if s.phase != PhaseLobby {
		s.msg = p.Name + " left"
	}
	s.mu.Unlock()
}

func cleanName(n string, existing map[int]*sPlayer) string {
	n = strings.TrimSpace(n)
	if n == "" {
		n = "Player"
	}
	if r := []rune(n); len(r) > MaxNameLen {
		n = string(r[:MaxNameLen])
	}
	base, k := n, 2
	for taken := true; taken; {
		taken = false
		for _, p := range existing {
			if p.Name == n {
				taken = true
				n = fmt.Sprintf("%s%d", base, k)
				k++
				break
			}
		}
	}
	return n
}

// StartRound splits everyone into two teams; catchers get the extra player when odd.
func (s *Server) StartRound() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != PhaseLobby || len(s.players) < 2 {
		return false
	}
	ids := make([]int, 0, len(s.players))
	for id := range s.players {
		ids = append(ids, id)
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	nCatch := (len(ids) + 1) / 2
	catchers, runners := ids[:nCatch], ids[nCatch:]

	place := func(list []int, x0, colW float64, team uint8) {
		rows := (len(list) + 1) / 2
		if len(list) <= 6 {
			rows = len(list)
		}
		for i, id := range list {
			p := s.players[id]
			col, row := 0, i
			if rows < len(list) {
				col, row = i/rows, i%rows
			}
			p.Team = team
			p.x = x0 + float64(col)*colW
			p.y = ArenaTop + float64(H-ArenaTop)*float64(row+1)/float64(rows+1)
			p.vx, p.vy = 0, 0
			p.energy = EnergyMax
			p.C, p.Esc, p.goalRight = 0, false, true
			p.swingT, p.dashT, p.cooldown, p.hitQueued = 0, 0, 0, false
			p.Dash = false
			if team == TeamCatcher {
				p.FX, p.FY = -1, 0
			} else {
				p.FX, p.FY = 1, 0
			}
		}
	}
	place(runners, ZoneW/2-28, 56, TeamRunner)
	place(catchers, W/2-30, 60, TeamCatcher)

	s.phase = PhaseCountdown
	s.timer = CountdownTime
	s.msg = ""
	return true
}

func (s *Server) tickLoop() {
	t := time.NewTicker(time.Second / TickRate)
	defer t.Stop()
	dt := 1.0 / TickRate
	n := 0
	for {
		select {
		case <-s.closed:
			return
		case <-t.C:
		}
		s.mu.Lock()
		s.step(dt)
		n++
		if n%SendEvery == 0 {
			s.broadcast()
		}
		s.mu.Unlock()
	}
}

func (s *Server) step(dt float64) {
	switch s.phase {
	case PhaseLobby:
		return
	case PhaseCountdown:
		s.timer -= dt
		if s.timer <= 0 {
			s.phase, s.timer = PhasePlaying, RoundTime
		}
		return
	case PhaseResults:
		s.timer -= dt
		if s.timer <= 0 {
			s.phase, s.msg = PhaseLobby, ""
			for _, p := range s.players {
				p.Team, p.Run, p.Swing = TeamNone, false, 0
			}
		}
		return
	}

	s.timer -= dt
	for _, p := range s.players {
		if p.Team != TeamNone {
			s.move(p, dt)
		}
	}
	for _, p := range s.players {
		if p.Team == TeamCatcher {
			s.swing(p, dt)
		}
	}

	active, escaped, catchers := 0, 0, 0
	for _, p := range s.players {
		switch {
		case p.Team == TeamCatcher:
			catchers++
		case p.Team == TeamRunner && p.Esc:
			escaped++
		case p.Team == TeamRunner:
			active++
		}
	}
	if active == 0 || catchers == 0 || s.timer <= 0 {
		s.phase, s.timer = PhaseResults, ResultsTime
		if escaped > 0 || (catchers == 0 && active > 0) {
			s.msg = fmt.Sprintf("RUNNERS WIN!  %d escaped", escaped)
		} else if active > 0 {
			s.msg = "CATCHERS WIN!  Time's up"
		} else {
			s.msg = "CATCHERS WIN!  Everyone got caught"
		}
	}
}

func (s *Server) move(p *sPlayer, dt float64) {
	var dx, dy float64
	if p.input&InUp != 0 {
		dy--
	}
	if p.input&InDown != 0 {
		dy++
	}
	if p.input&InLeft != 0 {
		dx--
	}
	if p.input&InRight != 0 {
		dx++
	}
	moving := dx != 0 || dy != 0
	if moving {
		l := math.Hypot(dx, dy)
		dx, dy = dx/l, dy/l
		p.FX, p.FY = float32(dx), float32(dy)
	}

	wantRun := p.input&InRun != 0 && moving
	if wantRun && (p.Run || p.energy >= EnergyMinGo) && p.energy > 0 {
		p.Run = true
		p.energy = math.Max(0, p.energy-EnergyDrain*dt)
	} else {
		p.Run = false
		p.energy = math.Min(EnergyMax, p.energy+EnergyRegen*dt)
	}

	if p.Team == TeamRunner {
		if p.cooldown > 0 {
			p.cooldown -= dt
		}
		if p.hitQueued {
			p.hitQueued = false
			if p.cooldown <= 0 && !p.Esc {
				ddx, ddy := float64(p.FX), float64(p.FY)
				if moving {
					ddx, ddy = dx, dy
				}
				p.dashT, p.cooldown = DashTime, DashCooldown
				p.vx, p.vy = ddx*DashSpeed, ddy*DashSpeed
			}
		}
	}

	if p.dashT > 0 { // dashing: keep the burst velocity, no steering
		p.dashT -= dt
		p.Dash = true
	} else {
		p.Dash = false
		speed := BaseSpeed
		if p.Run {
			speed *= RunMult
		}
		// Smooth acceleration with a little slide when keys are released.
		k := 9.0
		if !moving {
			k = 5.0
		}
		a := math.Min(1, k*dt)
		p.vx += (dx*speed - p.vx) * a
		p.vy += (dy*speed - p.vy) * a
	}
	p.x += p.vx * dt
	p.y += p.vy * dt

	minX, maxX := float64(Radius), float64(W-Radius)
	if p.Team == TeamCatcher { // catchers can't enter the safe zones
		minX, maxX = ZoneW+Radius, W-ZoneW-Radius
	}
	p.x = clamp(p.x, minX, maxX)
	p.y = clamp(p.y, ArenaTop+Radius, H-Radius)

	if p.Team == TeamRunner && !p.Esc {
		if p.goalRight && p.x >= W-ZoneW+Radius || !p.goalRight && p.x <= ZoneW-Radius {
			p.C++
			p.goalRight = !p.goalRight
			if p.C >= CrossToWin {
				p.Esc = true
				s.msg = p.Name + " escaped!"
			}
		}
	}
}

func (s *Server) swing(c *sPlayer, dt float64) {
	if c.cooldown > 0 {
		c.cooldown -= dt
	}
	if c.hitQueued {
		c.hitQueued = false
		if c.cooldown <= 0 {
			c.swingT, c.cooldown = SwingTime, HitCooldown
			c.hitDone = map[int]bool{}
		}
	}
	if c.swingT <= 0 {
		c.Swing = 0
		return
	}
	c.swingT -= dt
	c.Swing = float32(1 - math.Max(0, c.swingT)/SwingTime)
	if c.Swing == 0 {
		c.Swing = 0.01
	}
	for _, r := range s.players {
		if r.Team != TeamRunner || r.Esc || c.hitDone[r.ID] || inSafeZone(r.x) {
			continue
		}
		dx, dy := r.x-c.x, r.y-c.y
		d := math.Hypot(dx, dy)
		if d > HitReach {
			continue
		}
		// in front of the catcher (within ~75°) or basically touching
		if d < Radius*2+4 || (dx*float64(c.FX)+dy*float64(c.FY))/d > 0.26 {
			c.hitDone[r.ID] = true
			r.Team, r.Run = TeamCatcher, false
			r.vx, r.vy = 0, 0
			r.hitQueued, r.cooldown = false, HitCooldown
			r.dashT, r.Dash = 0, false
			r.x = clamp(r.x, ZoneW+Radius, W-ZoneW-Radius)
			s.msg = c.Name + " caught " + r.Name + "!"
		}
	}
	if c.swingT <= 0 {
		c.swingT = 0
	}
}

func inSafeZone(x float64) bool { return x < ZoneW || x > W-ZoneW }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func (s *Server) broadcast() {
	m := ServerMsg{T: "state", Phase: s.phase, Time: float32(s.timer), Msg: s.msg,
		Players: make([]PlayerState, 0, len(s.players))}
	for _, p := range s.players {
		ps := p.PlayerState
		ps.X, ps.Y, ps.E = float32(p.x), float32(p.y), uint8(p.energy)
		full := HitCooldown
		if p.Team == TeamRunner {
			full = DashCooldown
		}
		ps.CD = float32(math.Max(0, p.cooldown) / full)
		m.Players = append(m.Players, ps)
	}
	b, _ := json.Marshal(m)
	b = append(b, '\n')
	for _, p := range s.players {
		select {
		case p.out <- b:
		default: // slow client: drop this snapshot
		}
	}
}
