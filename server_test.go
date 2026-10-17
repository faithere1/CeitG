package main

import (
	"testing"
	"time"
)

func waitFor(t *testing.T, c *Client, ok func(ServerMsg) bool) ServerMsg {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, _, _, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if ok(st) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for state")
	return ServerMsg{}
}

func TestRound(t *testing.T) {
	s, err := StartServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var cs []*Client
	for _, n := range []string{"Ali", "Ayse", "Mehmet", "Ali"} {
		c, err := Connect("127.0.0.1", n)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		cs = append(cs, c)
	}
	st := waitFor(t, cs[0], func(m ServerMsg) bool { return len(m.Players) == 4 })
	names := map[string]bool{}
	for _, p := range st.Players {
		names[p.Name] = true
	}
	if !names["Ali2"] {
		t.Fatalf("duplicate name not renamed: %v", names)
	}

	// a 5th player makes it odd: catchers must get the extra one
	c5, _ := Connect("127.0.0.1", "Zeynep")
	defer c5.Close()
	waitFor(t, cs[0], func(m ServerMsg) bool { return len(m.Players) == 5 })
	if !s.StartRound() {
		t.Fatal("round did not start")
	}
	st = waitFor(t, cs[0], func(m ServerMsg) bool { return m.Phase == PhasePlaying })
	nr, nc := 0, 0
	for _, p := range st.Players {
		if p.Team == TeamRunner {
			nr++
		} else if p.Team == TeamCatcher {
			nc++
		}
	}
	if nc != 3 || nr != 2 {
		t.Fatalf("want 3 catchers / 2 runners, got %d / %d", nc, nr)
	}

	// Put one catcher next to a runner in the open field and swing.
	s.mu.Lock()
	var catcher, runner, other *sPlayer
	for _, p := range s.players {
		switch {
		case p.Team == TeamCatcher && catcher == nil:
			catcher = p
		case p.Team == TeamRunner && runner == nil:
			runner = p
		case p.Team == TeamRunner:
			other = p
		}
	}
	runner.x, runner.y = 640, 400
	catcher.x, catcher.y, catcher.FX, catcher.FY = 590, 400, 1, 0
	catcher.hitQueued = true
	s.mu.Unlock()

	time.Sleep(300 * time.Millisecond)
	s.mu.Lock()
	if runner.Team != TeamCatcher {
		t.Fatal("runner was not converted by the hit")
	}
	// Second runner: walk 3 crossings by teleporting into the goal zones.
	for i := 0; i < 3; i++ {
		if other.goalRight {
			other.x = W - Radius
		} else {
			other.x = Radius
		}
		s.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		s.mu.Lock()
	}
	esc := other.Esc
	s.mu.Unlock()
	if !esc {
		t.Fatal("runner did not escape after 3 crossings")
	}
	st = waitFor(t, cs[0], func(m ServerMsg) bool { return m.Phase == PhaseResults })
	if st.Msg[:7] != "RUNNERS" {
		t.Fatalf("unexpected result %q", st.Msg)
	}
}

func TestDash(t *testing.T) {
	s := &Server{players: map[int]*sPlayer{}, phase: PhasePlaying, timer: RoundTime}
	r := &sPlayer{x: 400, y: 400, energy: EnergyMax, goalRight: true}
	r.Team, r.FX = TeamRunner, 1
	s.players[1] = r
	walk := func(sec float64) float64 {
		x0 := r.x
		for i := 0; i < int(sec*TickRate); i++ {
			s.move(r, 1.0/TickRate)
		}
		return r.x - x0
	}

	r.input = InRight
	walk(1) // reach full walking speed
	normal := walk(DashTime)
	r.hitQueued = true
	dashed := walk(DashTime)
	if dashed < normal*2.5 {
		t.Fatalf("dash too weak: %.0fpx vs %.0fpx walking", dashed, normal)
	}
	if r.cooldown <= 0 {
		t.Fatal("dash did not start a cooldown")
	}
	// a second dash right away is blocked by the cooldown
	walk(0.5)
	r.hitQueued = true
	if again := walk(DashTime); again > normal*1.2 {
		t.Fatalf("dashed again during cooldown (%.0fpx)", again)
	}
	walk(DashCooldown)
	r.x = 400 // back away from the wall before measuring
	r.hitQueued = true
	if again := walk(DashTime); again < normal*2.5 {
		t.Fatalf("dash not available after cooldown (%.0fpx)", again)
	}
}
