package agario

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"unicode/utf16"

	"golang.org/x/net/websocket"
)

type Connection struct {
	Addr net.Addr

	ws     *websocket.Conn
	buffer *bytes.Reader
}

func (c *Connection) connect(addr net.Addr) error {
	//c.Addr, _ = net.ResolveTCPAddr("tcp", "167.114.174.64:1504")
	c.Addr = addr
	var err error

	c.ws, err = websocket.Dial("ws://"+c.Addr.String()+"/", "", "http://agar.io")
	if err != nil {
		return err
	}

	_, err = c.ws.Write([]byte{254, 4, 0, 0, 0})
	if err != nil {
		return err
	}

	err = websocket.Message.Send(c.ws, []byte{255, 40, 40, 40, 40})
	if err != nil {
		return err
	}

	return nil
}

type targetData struct {
	ID byte
	X  float64
	Y  float64
}

var ErrUnknownMessage = errors.New("agario: received unknown message type")

func (c *Connection) Read() (msg Message, err error) {
	defer func() {
		if recoverVal := recover(); recoverVal != nil {
			msg = nil
			if recoverErr, ok := recoverVal.(error); ok {
				err = recoverErr
			} else {
				err = fmt.Errorf("recovered from panic: %v", recoverVal)
			}
		}
	}()

	var msgBytes []byte
	err = websocket.Message.Receive(c.ws, &msgBytes)
	if err != nil {
		return
	}
	c.buffer = bytes.NewReader(msgBytes)
	defer func() {
		if c.buffer.Len() > 0 {
			log.Printf("WARNING: Not all bytes read in message")
		}
		c.buffer = nil
	}()

	messageType := c.mustReadByte()

	if messageType == 0xF0 {
		//log.Printf("agario: skipping first 5 bytes (found 0xF0 as the type)")
		c.buffer.Seek(4, 1)
		messageType = c.mustReadByte()
	}

	switch messageType {
	case 0x10: // update
		msg := new(MessageUpdate)

		numCellsDestroyed := c.mustReadUint16()
		msg.Destroyed = make([]*CellDestroyed, numCellsDestroyed)

		for i := uint16(0); i < numCellsDestroyed; i++ {
			msg.Destroyed[i] = &CellDestroyed{
				Attacker: c.mustReadUint32(),
				Victim:   c.mustReadUint32(),
			}
		}

		for {
			id := c.mustReadUint32()
			if id == 0 {
				break
			}

			x := c.mustReadInt16()
			y := c.mustReadInt16()
			size := c.mustReadInt16()

			colorR := c.mustReadByte()
			colorG := c.mustReadByte()
			colorB := c.mustReadByte()

			pointFlags := c.mustReadByte()

			pointIsVirus := pointFlags&1 == 1
			pointIsAgitated := pointFlags&16 == 16
			if size <= 16 {
				flags := make([]byte, 0, 8)
				for i := byte(1); i <= 8; i++ {
					if pointFlags&i != i {
						continue
					}

					flags = append(flags, i)
				}

				if len(flags) > 0 {
					log.Printf("agario: Flags for %d (%d): %v", id, size, flags)
				}
			}
			if pointFlags&2 == 2 {
				log.Printf("agario: skipping 4 bytes (flag has #2 bit set)")
				c.mustReadBytes(4)
			}
			if pointFlags&4 == 4 {
				log.Printf("skipping 8 bytes (flag has #4 bit set)")
				c.mustReadBytes(8)
			}
			if pointFlags&8 == 8 {
				log.Printf("skipping 16 bytes (flag has #8 bit set)")
				c.mustReadBytes(16)
			}

			pointName := c.mustReadString()

			msg.Updated = append(msg.Updated, &CellUpdate{
				ID: id,

				Point: Point{
					X: x,
					Y: y,
				},

				Size: size,

				R: colorR,
				G: colorG,
				B: colorB,

				Virus:    pointIsVirus,
				Agitated: pointIsAgitated,

				Name: pointName,
			})
		}

		numClean := c.mustReadUint32()
		msg.Clean = make([]uint32, numClean)
		for i := uint32(0); i < numClean; i++ {
			id := c.mustReadUint32()
			msg.Clean[i] = id
		}

		return msg, nil
	case 0x20: // my ID
		return &MessageMyID{
			MyCell: c.mustReadUint32(),
		}, nil
	case 0x31: // Leaderboard
		numItems := c.mustReadUint32()

		leaderboard := &MessageLeaderboard{
			Items: make([]*LeaderboardItem, numItems),
		}

		for seek := uint32(0); seek < numItems; seek++ {
			leaderboard.Items[seek] = &LeaderboardItem{
				ID:   c.mustReadUint32(),
				Name: c.mustReadString(),
			}
		}
		return leaderboard, nil
	case 0x32:
		n := c.mustReadUint32()
		if n != 3 {
			return messageType, ErrUnknownMessage
		}

		msg := &MessageTeamLeaderboard{
			Red:   c.mustReadFloat32(),
			Green: c.mustReadFloat32(),
			Blue:  c.mustReadFloat32(),
		}

		return msg, nil
	case 0x40:
		return &MessageCanvasSize{
			Left:   c.mustReadFloat64(),
			Top:    c.mustReadFloat64(),
			Right:  c.mustReadFloat64(),
			Bottom: c.mustReadFloat64(),
		}, nil
	case 0x48: // ASCII "H"
		// The server sends "HelloHelloHello" as its first message
		b := c.mustReadBytes(14)
		if string(b) == "elloHelloHello" {
			return new(MessageHello), nil
		}
		fallthrough
	default:
		return messageType, ErrUnknownMessage
	}
}

func (c *Connection) Close() error {
	return c.ws.Close()
}

func (c *Connection) SendNickname(nickname string) error {
	msg := make([]byte, 1+2*len(nickname))
	encodeString(msg[1:], nickname)
	err := websocket.Message.Send(c.ws, msg)
	return err
}

func (c *Connection) SendTarget(x, y float64) error {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, &targetData{0x10, x, y})
	if err != nil {
		return err
	}

	buf.Write([]byte{0, 0, 0, 0})

	err = websocket.Message.Send(c.ws, buf.Bytes())
	if err != nil {
		return err
	}

	//_, err = c.obuffer.Write([]byte{0, 0, 0, 0})
	return err
}

func (c *Connection) SendSplit() error {
	return websocket.Message.Send(c.ws, []byte{0x11})
}

func (c *Connection) mustReadByte() byte {
	b, err := c.buffer.ReadByte()
	if err != nil {
		panic(err)
	}
	return b
}

func (c *Connection) mustReadBytes(n int) []byte {
	b := make([]byte, n)
	_, err := c.buffer.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}

func (c *Connection) mustReadUint16() uint16 {
	b := c.mustReadBytes(2)
	return binary.LittleEndian.Uint16(b)
}

func (c *Connection) mustReadInt16() int16 {
	b := c.mustReadBytes(2)
	return int16(binary.LittleEndian.Uint16(b))
}

func (c *Connection) mustReadUint32() uint32 {
	b := c.mustReadBytes(4)
	return binary.LittleEndian.Uint32(b)
}

func (c *Connection) mustReadFloat32() float32 {
	b := c.mustReadBytes(4)
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

func (c *Connection) mustReadFloat64() float64 {
	b := c.mustReadBytes(8)
	return math.Float64frombits(binary.LittleEndian.Uint64(b))
}

func (c *Connection) mustReadString() string {
	var chars []uint16

	for {
		b := c.mustReadBytes(2)
		c := binary.LittleEndian.Uint16(b)
		if c == 0 {
			break
		}

		chars = append(chars, c)
	}

	str := string(utf16.Decode(chars))

	return str
}

func encodeString(dst []byte, str string) {
	encoded := utf16.Encode([]rune(str))
	for i, b := range encoded {
		binary.LittleEndian.PutUint16(dst[2*i:], b)
	}
}
