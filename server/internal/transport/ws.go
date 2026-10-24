// Package transport exposes the room over WebSocket.
package transport

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"killa/server/internal/game"
	"killa/server/internal/protocol"
)

var errConnClosed = errors.New("connection closed")

const (
	joinTimeout          = 5 * time.Second
	idleTimeout          = 120 * time.Second
	maxMessage           = 4096
	maxConnections       = 256
	maxMessagesPerSecond = 400
)

// Joiner places a connection into a room.
type Joiner interface {
	Join(peer game.Peer, name string, weapon ...string) (*game.Room, error)
}

// Server upgrades HTTP requests to WebSocket connections and feeds a room.
type Server struct {
	joiner   Joiner
	allowed  []string
	upgrader websocket.Upgrader
	conns    chan struct{}
}

func NewServer(joiner Joiner, allowedOrigins []string) *Server {
	s := &Server{
		joiner:  joiner,
		allowed: allowedOrigins,
		conns:   make(chan struct{}, maxConnections),
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return originAllowed(r, s.allowed)
		},
	}
	return s
}

func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	select {
	case s.conns <- struct{}{}:
		defer func() { <-s.conns }()
	default:
		http.Error(w, "server busy", http.StatusServiceUnavailable)
		return
	}

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := newConn(ws)
	go conn.writeLoop()

	ws.SetReadLimit(maxMessage)
	_ = ws.SetReadDeadline(time.Now().Add(joinTimeout))

	var join protocol.Join
	if err := ws.ReadJSON(&join); err != nil || join.Type != "join" {
		_ = conn.Send(protocol.Error{Type: "error", Message: "expected join message"})
		time.Sleep(50 * time.Millisecond)
		_ = conn.Close()
		return
	}

	room, err := s.joiner.Join(conn, join.Name, join.Weapon)
	if err != nil {
		log.Printf("killa: join rejected: %v", err)
		_ = conn.Send(protocol.Error{Type: "error", Message: err.Error()})
		time.Sleep(50 * time.Millisecond)
		_ = conn.Close()
		return
	}
	defer room.Leave(conn)
	defer conn.Close()

	windowStart := time.Now()
	windowCount := 0

	for {
		_ = ws.SetReadDeadline(time.Now().Add(idleTimeout))
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}

		now := time.Now()
		if now.Sub(windowStart) >= time.Second {
			windowStart = now
			windowCount = 0
		}
		windowCount++
		if windowCount > maxMessagesPerSecond {
			_ = conn.Send(protocol.Error{Type: "error", Message: "rate limit exceeded"})
			time.Sleep(20 * time.Millisecond)
			return
		}

		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}

		switch env.Type {
		case "input":
			var in protocol.Input
			if err := json.Unmarshal(raw, &in); err == nil {
				room.Input(conn, in)
			}
		case "shoot":
			var shot protocol.Shoot
			if err := json.Unmarshal(raw, &shot); err == nil {
				room.Shoot(conn, shot.Yaw, shot.Pitch)
			}
		case "loadout":
			var loadout protocol.Loadout
			if err := json.Unmarshal(raw, &loadout); err == nil {
				room.SetLoadout(conn, loadout.Weapon)
			}
		case "reload":
			room.Reload(conn)
		case "ping":
			var p protocol.Ping
			if err := json.Unmarshal(raw, &p); err == nil {
				_ = conn.Send(protocol.Pong{Type: "pong", T: p.T})
			}
		}
	}
}

// Conn is a websocket-backed game.Peer with a bounded, drop-on-full send queue.
type Conn struct {
	ws   *websocket.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

func newConn(ws *websocket.Conn) *Conn {
	return &Conn{
		ws:   ws,
		send: make(chan []byte, 64),
		done: make(chan struct{}),
	}
}

func (c *Conn) Send(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}

	select {
	case <-c.done:
		return errConnClosed
	default:
	}

	select {
	case c.send <- payload:
		return nil
	case <-c.done:
		return errConnClosed
	default:
		// Slow consumer: drop this frame instead of stalling the room.
		return nil
	}
}

func (c *Conn) Close() error {
	c.once.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
	return nil
}

func (c *Conn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case payload := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

func originAllowed(r *http.Request, allowed []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, item := range allowed {
		if strings.EqualFold(strings.TrimSpace(item), origin) {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
