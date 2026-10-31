package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"killa/server/internal/game"
	"killa/server/internal/protocol"
	"killa/server/internal/store"
)

type singleRoom struct {
	room *game.Room
}

func (s singleRoom) Join(peer game.Peer, name string, weapon ...string) (*game.Room, error) {
	if err := s.room.Join(peer, name, weapon...); err != nil {
		return nil, err
	}
	return s.room, nil
}

func (s singleRoom) JoinProfile(peer game.Peer, name, weapon, profileID string) (*game.Room, error) {
	if err := s.room.JoinProfile(peer, name, weapon, profileID); err != nil {
		return nil, err
	}
	return s.room, nil
}

type stubResolver struct{}

func (stubResolver) Resolve(token string) (*store.Profile, error) {
	if token == "good" {
		return &store.Profile{ID: "profile-1", DisplayName: "Alice", Loadout: "smg"}, nil
	}
	return nil, store.ErrNotFound
}

func newTestServer(t *testing.T) (string, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	room := game.NewRoom("test-room", game.NewWarehouse())
	go room.Run(ctx)

	api := NewServer(singleRoom{room: room}, nil)
	ts := httptest.NewServer(http.HandlerFunc(api.HandleWS))

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	cleanup := func() {
		ts.Close()
		cancel()
	}
	return url, cleanup
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func readJSON[T any](t *testing.T, conn *websocket.Conn, timeout time.Duration) T {
	t.Helper()
	var value T
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	if err := conn.ReadJSON(&value); err != nil {
		t.Fatalf("read: %v", err)
	}
	return value
}

func TestMovementReplication(t *testing.T) {
	url, cleanup := newTestServer(t)
	defer cleanup()

	c1 := dial(t, url)
	defer c1.Close()
	c2 := dial(t, url)
	defer c2.Close()

	if err := c1.WriteJSON(protocol.Join{Type: "join", Name: "one"}); err != nil {
		t.Fatalf("join one: %v", err)
	}
	first := readJSON[protocol.Welcome](t, c1, 3*time.Second)
	if first.Type != "welcome" {
		t.Fatalf("first message = %q, want welcome", first.Type)
	}
	if len(first.Map.Crates) == 0 || len(first.Map.Spawns) != 8 {
		t.Fatalf("unexpected map payload: %d crates, %d spawns", len(first.Map.Crates), len(first.Map.Spawns))
	}

	if err := c2.WriteJSON(protocol.Join{Type: "join", Name: "two"}); err != nil {
		t.Fatalf("join two: %v", err)
	}
	second := readJSON[protocol.Welcome](t, c2, 3*time.Second)
	if second.Type != "welcome" {
		t.Fatalf("second join got %q", second.Type)
	}
	if first.Team == second.Team {
		t.Fatalf("both players on team %d", first.Team)
	}

	for i := 1; i <= 30; i++ {
		in := protocol.Input{Type: "input", Seq: uint32(i), Dt: 0.033, Forward: 1, Yaw: 0}
		if err := c1.WriteJSON(in); err != nil {
			t.Fatalf("input %d: %v", i, err)
		}
	}

	moved := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := readJSON[protocol.Snapshot](t, c2, 2*time.Second)
		if len(snap.Players) != 2 {
			continue
		}
		for _, p := range snap.Players {
			if p.ID == first.PlayerID && p.Z < 14 {
				moved = true
			}
		}
		if moved {
			break
		}
	}
	if !moved {
		t.Fatal("movement was not replicated to the second player")
	}

	var ack uint32
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := readJSON[protocol.Snapshot](t, c1, 2*time.Second)
		if snap.Ack > ack {
			ack = snap.Ack
		}
		if ack >= 30 {
			break
		}
	}
	if ack < 30 {
		t.Fatalf("ack = %d, want >= 30", ack)
	}
}

func TestRejectsNonJoinFirstMessage(t *testing.T) {
	url, cleanup := newTestServer(t)
	defer cleanup()

	conn := dial(t, url)
	defer conn.Close()

	if err := conn.WriteJSON(protocol.Input{Type: "input", Seq: 1}); err != nil {
		t.Fatalf("write: %v", err)
	}

	var msg protocol.Error
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read: %v", err)
	}
	if msg.Type != "error" {
		t.Fatalf("type = %q, want error", msg.Type)
	}
}

func TestRateLimitClosesConnection(t *testing.T) {
	url, cleanup := newTestServer(t)
	defer cleanup()

	conn := dial(t, url)
	defer conn.Close()

	if err := conn.WriteJSON(protocol.Join{Type: "join", Name: "spam"}); err != nil {
		t.Fatalf("join: %v", err)
	}
	_ = readJSON[protocol.Welcome](t, conn, 3*time.Second)

	for i := 0; i < 450; i++ {
		if err := conn.WriteJSON(protocol.Ping{Type: "ping", T: float64(i)}); err != nil {
			break
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var msg map[string]any
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		if msg["type"] == "error" {
			return
		}
	}
	t.Fatal("expected a rate limit error")
}

func TestJoinRequiresSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	room := game.NewRoom("auth-room", game.NewWarehouse())
	go room.Run(ctx)

	api := NewServer(singleRoom{room: room}, nil)
	api.SetSessionResolver(stubResolver{})
	ts := httptest.NewServer(http.HandlerFunc(api.HandleWS))
	defer ts.Close()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// No cookie: the join is rejected before reaching the room.
	anon, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := anon.WriteJSON(protocol.Join{Type: "join", Name: "nobody"}); err != nil {
		t.Fatal(err)
	}
	rejection := readJSON[protocol.Error](t, anon, 3*time.Second)
	if rejection.Type != "error" || !strings.Contains(rejection.Message, "登录") {
		t.Fatalf("rejection = %+v", rejection)
	}
	_ = anon.Close()

	// With a session cookie the profile name/loadout replace client-supplied values.
	header := http.Header{}
	header.Set("Cookie", "killa_session=good")
	authed, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatal(err)
	}
	defer authed.Close()
	if err := authed.WriteJSON(protocol.Join{Type: "join", Name: "spoofed", Weapon: "ar"}); err != nil {
		t.Fatal(err)
	}
	welcome := readJSON[protocol.Welcome](t, authed, 3*time.Second)
	if welcome.Type != "welcome" {
		t.Fatalf("welcome = %+v", welcome)
	}
	snap := readJSON[protocol.Snapshot](t, authed, 3*time.Second)
	var found bool
	for _, p := range snap.Players {
		if p.ID == welcome.PlayerID {
			found = true
			if p.Name != "Alice" || p.Weapon != "smg" {
				t.Fatalf("profile not applied: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("self not found in snapshot")
	}
}
