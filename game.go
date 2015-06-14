package agario

import (
	"image/color"
	"log"
	"math"
	"net"
	"sync"
	"time"

	"github.com/go-gl/mathgl/mgl32"
)

func NewGame(c *Connection) *Game {
	return &Game{
		Mutex: new(sync.Mutex),

		c: c,

		MyIDs: make(map[uint32]struct{}),
		Cells: make(map[uint32]*Cell),
	}
}

type TickFunc func(g *Game)

type Game struct {
	*sync.Mutex

	c *Connection

	Leaderboard []*LeaderboardItem

	targetX, targetY float64

	Board *MessageCanvasSize

	MyIDs map[uint32]struct{}
	Cells map[uint32]*Cell
}

func (g *Game) RunOnce(timeout bool) bool {
	if timeout {
		g.c.ws.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	} else {
		g.c.ws.SetReadDeadline(time.Time{})
	}
	msgInterface, err := g.c.Read()

	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() || netErr.Temporary() {
			return false
		}
		panic(netErr)
	} else if err == ErrUnknownMessage {
		log.Printf("%s: %v", err, msgInterface)
		panic(err)
	} else if err != nil {
		panic(err)
	}

	g.Lock()
	defer g.Unlock()
	switch msg := msgInterface.(type) {
	case *MessageUpdate:
		for _, dest := range msg.Destroyed {
			attacker := g.Cells[dest.Attacker]
			victim := g.Cells[dest.Victim]
			if attacker != nil && victim != nil {
				log.Printf("%q (%d) destroyed %q (%d)", attacker.Name, attacker.Size, victim.Name, victim.Size)
			}
			delete(g.Cells, dest.Victim)
		}

		for _, update := range msg.Updated {
			cell, ok := g.Cells[update.ID]
			if !ok {
				g.Cells[update.ID] = &Cell{
					ID:   update.ID,
					Name: update.Name,

					Position: mgl32.Vec2{float32(update.X), float32(update.Y)},
					Size:     int32(update.Size),

					Color: color.RGBA{update.R, update.G, update.B, 255},

					IsVirus: update.Virus,
				}
				continue
			}

			cell.IsVirus = update.Virus

			newPosition := mgl32.Vec2{float32(update.X), float32(update.Y)}
			oldPosition := cell.Position

			if !oldPosition.ApproxEqual(newPosition) {
				cell.Position = newPosition

				positionDifference := cell.Position.Sub(oldPosition)
				cell.Heading = positionDifference.Normalize()
			} else {
				cell.Position = newPosition
				cell.Heading = mgl32.Vec2{0, 0}
			}

			cell.Size = int32(update.Size)

			if update.Name != "" {
				cell.Name = update.Name
			}
		}

		for _, id := range msg.Clean {
			delete(g.Cells, id)
			if _, myCell := g.MyIDs[id]; myCell {
				delete(g.MyIDs, id)

				if len(g.MyIDs) == 0 {
					g.targetX = 0
					g.targetY = 0
				}
			}
		}
	case *MessageCanvasSize:
		g.Board = msg
	case *MessageLeaderboard:
		g.Leaderboard = msg.Items
	case *MessageMyID:
		g.MyIDs[msg.MyCell] = struct{}{}
	default:
		log.Printf("Unhandled message: %#v", msg)
	}

	return true
}

func (g *Game) SetTargetPos(x_, y_ float32) {
	x, y := float64(x_), float64(y_)
	if x == g.targetX && y == g.targetY {
		return
	}

	g.targetX, g.targetY = x, y
	if err := g.c.SendTarget(x, y); err != nil {
		panic(err)
	}
}

func (g *Game) Split() {
	if err := g.c.SendSplit(); err != nil {
		panic(err)
	}
}

func (g *Game) SendNickname(nickname string) {
	err := g.c.SendNickname(nickname)
	if err != nil {
		panic(err)
	}
}

func (g *Game) Close() error {
	return g.c.Close()
}

type Cell struct {
	ID   uint32
	Name string

	Position mgl32.Vec2 // Current position
	Heading  mgl32.Vec2 // Heading as normalized vector

	Size int32

	Color color.Color

	IsVirus bool
}

func (c *Cell) Speed() float32 {
	return 745.28 * float32(math.Pow(float64(c.Size), -0.222)) * 50 / 1000
}

func (c *Cell) SplitDistance() float32 {
	return square((4*(40+(c.Speed()*4)) + (float32(c.Size) * 1.75)) + 100)
}

func square(a float32) float32 {
	return a * a
}
