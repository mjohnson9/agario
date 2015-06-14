package agario

type Message interface{}

type Point struct {
	X, Y int16
}

type CellDestroyed struct {
	Attacker uint32
	Victim   uint32
}

type CellUpdate struct {
	ID uint32

	Point

	Size int16

	R byte
	G byte
	B byte

	Virus    bool
	Agitated bool

	Name string
}

type MessageUpdate struct {
	Destroyed []*CellDestroyed
	Updated   []*CellUpdate
	Clean     []uint32
}

type MessageMyID struct {
	MyCell uint32
}

type LeaderboardItem struct {
	ID   uint32
	Name string
}

type MessageLeaderboard struct {
	Items []*LeaderboardItem
}

type MessageTeamLeaderboard struct {
	Red   float32
	Green float32
	Blue  float32
}

type MessageCanvasSize struct {
	Left   float64
	Bottom float64
	Right  float64
	Top    float64
}

type MessageHello struct{}
