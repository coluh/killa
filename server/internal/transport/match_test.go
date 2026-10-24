package transport

import (
	"math"
	"testing"
	"time"

	"killa/server/internal/protocol"
)

func TestMatchStartsAndScores(t *testing.T) {
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

	live := waitSnapshot(t, shooter, 7*time.Second, func(snap protocol.Snapshot) bool {
		return snap.Match.State == "live"
	})
	if live.Match.SecondsLeft < 500 {
		t.Fatalf("secondsLeft = %v, want close to 600", live.Match.SecondsLeft)
	}
	if live.Match.Score != [2]int{0, 0} {
		t.Fatalf("score = %v, want 0:0", live.Match.Score)
	}

	// 比赛开始会重置位置并给 1.5s 出生保护，等保护结束再横移对枪。
	time.Sleep(1600 * time.Millisecond)
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

	scored := waitSnapshot(t, shooter, 5*time.Second, func(snap protocol.Snapshot) bool {
		return snap.Match.State == "live" && snap.Match.Score[0] == 1
	})

	if scored.Match.Score != [2]int{1, 0} {
		t.Fatalf("score = %v, want 1:0", scored.Match.Score)
	}
	killer, ok := findPlayer(scored, w1.PlayerID)
	if !ok || killer.Kills != 1 {
		t.Fatalf("killer stats = %+v", killer)
	}
	if _, ok := findPlayer(scored, w2.PlayerID); !ok {
		t.Fatal("victim missing from snapshot")
	}
}
