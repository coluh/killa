package transport

import (
	"math"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"killa/server/internal/protocol"
)

func waitSnapshot(t *testing.T, conn *websocket.Conn, timeout time.Duration, pred func(protocol.Snapshot) bool) protocol.Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var snap protocol.Snapshot
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := conn.ReadJSON(&snap); err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		if snap.Type == "snapshot" && pred(snap) {
			return snap
		}
	}
	t.Fatal("timed out waiting for snapshot")
	return protocol.Snapshot{}
}

func findPlayer(snap protocol.Snapshot, id string) (protocol.Player, bool) {
	for _, p := range snap.Players {
		if p.ID == id {
			return p, true
		}
	}
	return protocol.Player{}, false
}

func TestShootingKillAndRespawn(t *testing.T) {
	url, cleanup := newTestServer(t)
	defer cleanup()

	shooter := dial(t, url)
	defer shooter.Close()
	victim := dial(t, url)
	defer victim.Close()

	if err := shooter.WriteJSON(protocol.Join{Type: "join", Name: "shooter"}); err != nil {
		t.Fatalf("join shooter: %v", err)
	}
	w1 := readJSON[protocol.Welcome](t, shooter, 3*time.Second)

	if err := victim.WriteJSON(protocol.Join{Type: "join", Name: "victim"}); err != nil {
		t.Fatalf("join victim: %v", err)
	}
	w2 := readJSON[protocol.Welcome](t, victim, 3*time.Second)

	// 两个出生点都在 x = -2，先各自向左横移到 x ≈ -3.58，绕开中路掩体。
	for i := 1; i <= 10; i++ {
		in := protocol.Input{Type: "input", Seq: uint32(i), Dt: 0.033, Right: -1, Yaw: 0}
		if err := shooter.WriteJSON(in); err != nil {
			t.Fatalf("shooter input: %v", err)
		}
	}
	for i := 1; i <= 10; i++ {
		in := protocol.Input{Type: "input", Seq: uint32(i), Dt: 0.033, Right: 1, Yaw: math.Pi}
		if err := victim.WriteJSON(in); err != nil {
			t.Fatalf("victim input: %v", err)
		}
	}
	time.Sleep(400 * time.Millisecond)

	for i := 0; i < 6; i++ {
		if err := shooter.WriteJSON(protocol.Shoot{Type: "shoot", Yaw: 0, Pitch: 0}); err != nil {
			t.Fatalf("shoot: %v", err)
		}
		time.Sleep(130 * time.Millisecond)
	}

	dead := waitSnapshot(t, victim, 5*time.Second, func(snap protocol.Snapshot) bool {
		p, ok := findPlayer(snap, w2.PlayerID)
		return ok && !p.Alive
	})
	deadVictim, _ := findPlayer(dead, w2.PlayerID)
	if deadVictim.Deaths != 1 {
		t.Fatalf("deaths = %d, want 1", deadVictim.Deaths)
	}
	if deadVictim.RespawnIn <= 0 {
		t.Fatalf("respawnIn = %v, want > 0", deadVictim.RespawnIn)
	}

	kills := waitSnapshot(t, shooter, 5*time.Second, func(snap protocol.Snapshot) bool {
		p, ok := findPlayer(snap, w1.PlayerID)
		return ok && p.Kills == 1
	})
	if killer, _ := findPlayer(kills, w1.PlayerID); killer.Kills != 1 {
		t.Fatalf("kills = %d, want 1", killer.Kills)
	}

	time.Sleep(3200 * time.Millisecond)
	respawned := waitSnapshot(t, victim, 5*time.Second, func(snap protocol.Snapshot) bool {
		p, ok := findPlayer(snap, w2.PlayerID)
		return ok && p.Alive
	})
	back, _ := findPlayer(respawned, w2.PlayerID)
	if back.HP != 100 {
		t.Fatalf("respawn hp = %d, want 100", back.HP)
	}
	if back.Z > -10 {
		t.Fatalf("respawn z = %v, want around -16", back.Z)
	}
}
