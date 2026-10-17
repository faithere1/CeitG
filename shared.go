package main

// Game world is a fixed 1280x720 logical screen; the window scales it.
const (
	W         = 1280
	H         = 720
	ArenaTop  = 52  // HUD strip height
	ZoneW     = 170 // width of each safe zone (left & right)
	Radius    = 24
	StickLen  = 34
	GamePort  = 47800
	DiscPort  = 47801
	TickRate  = 60
	SendEvery = 2 // state snapshots at 30 Hz

	BaseSpeed   = 230.0
	RunMult     = 1.7
	EnergyMax   = 100.0
	EnergyDrain = 38.0 // per second while running
	EnergyRegen = 16.0 // per second while not running
	EnergyMinGo = 12.0 // needed to start a run

	SwingTime   = 0.22
	HitCooldown = 0.9
	HitReach    = Radius + StickLen + Radius*0.7

	CountdownTime = 3.0
	RoundTime     = 150.0
	ResultsTime   = 7.0
	CrossToWin    = 3
	MaxNameLen    = 8
)

// Input bits.
const (
	InUp uint8 = 1 << iota
	InDown
	InLeft
	InRight
	InRun
	InHit
)

// Teams.
const (
	TeamNone    uint8 = iota // lobby / spectator
	TeamRunner               // must cross the field 3 times
	TeamCatcher              // tags runners with the stick
)

// Phases.
const (
	PhaseLobby uint8 = iota
	PhaseCountdown
	PhasePlaying
	PhaseResults
)

type ClientMsg struct {
	T    string `json:"t"` // "hello" | "in"
	Name string `json:"n,omitempty"`
	K    uint8  `json:"k,omitempty"`
}

type PlayerState struct {
	ID    int     `json:"i"`
	Name  string  `json:"n"`
	X     float32 `json:"x"`
	Y     float32 `json:"y"`
	FX    float32 `json:"fx"` // facing direction
	FY    float32 `json:"fy"`
	Team  uint8   `json:"t"`
	Run   bool    `json:"r,omitempty"`
	Swing float32 `json:"s,omitempty"` // 0 = idle, (0..1] swing progress
	CD    float32 `json:"cd,omitempty"`
	E     uint8   `json:"e"` // energy 0..100
	C     uint8   `json:"c"` // crossings done
	Esc   bool    `json:"es,omitempty"`
	Host  bool    `json:"h,omitempty"`
}

type ServerMsg struct {
	T       string        `json:"t"` // "welcome" | "state"
	ID      int           `json:"id,omitempty"`
	Phase   uint8         `json:"p"`
	Time    float32       `json:"tm"`
	Msg     string        `json:"m,omitempty"`
	Players []PlayerState `json:"ps,omitempty"`
}
