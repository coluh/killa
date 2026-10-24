package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"killa/server/internal/game"
	"killa/server/internal/protocol"
)

func newBotTestServer(t *testing.T) (string, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	room := game.NewRoom("bots-test", game.NewWarehouse())
	room.SetAutoBots(true)
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

func countTeams(snap protocol.Snapshot) (int, int, int) {
	humans, bots, perTeam := 0, 0, [2]int{}
	for _, p := range snap.Players {
		if strings.HasPrefix(p.Name, "BOT-") {
			bots++
		} else {
			humans++
		}
		if p.Team == 0 || p.Team == 1 {
			perTeam[p.Team]++
		}
	}
	return humans, bots, perTeam[0]*10 + perTeam[1]
}

func TestBotsFillAndYield(t *testing.T) {
	url, cleanup := newBotTestServer(t)
	defer cleanup()

	c1 := dial(t, url)
	defer c1.Close()
	if err := c1.WriteJSON(protocol.Join{Type: "join", Name: "human1"}); err != nil {
		t.Fatalf("join human1: %v", err)
	}
	_ = readJSON[protocol.Welcome](t, c1, 3*time.Second)

	full := waitSnapshot(t, c1, 10*time.Second, func(snap protocol.Snapshot) bool {
		return len(snap.Players) == 8 && snap.Match.State == "live"
	})

	humans, bots, teams := countTeams(full)
	if humans != 1 || bots != 7 {
		t.Fatalf("humans/bots = %d/%d, want 1/7", humans, bots)
	}
	if teams != 44 {
		t.Fatalf("team sizes = %d, want 4/4", teams)
	}

	// bots should be moving out of spawn once the match is live; read forward
	// through queued snapshots until one shows movement.
	waitSnapshot(t, c1, 5*time.Second, func(snap protocol.Snapshot) bool {
		for _, p := range snap.Players {
			if !strings.HasPrefix(p.Name, "BOT-") {
				continue
			}
			for _, before := range full.Players {
				if before.ID == p.ID && (abs(before.X-p.X) > 0.5 || abs(before.Z-p.Z) > 0.5) {
					return true
				}
			}
		}
		return false
	})

	c2 := dial(t, url)
	defer c2.Close()
	if err := c2.WriteJSON(protocol.Join{Type: "join", Name: "human2"}); err != nil {
		t.Fatalf("join human2: %v", err)
	}
	_ = readJSON[protocol.Welcome](t, c2, 3*time.Second)

	yielded := waitSnapshot(t, c1, 5*time.Second, func(snap protocol.Snapshot) bool {
		humans, bots, _ := countTeams(snap)
		return len(snap.Players) == 8 && humans == 2 && bots == 6
	})

	humans, bots, teams = countTeams(yielded)
	if humans != 2 || bots != 6 {
		t.Fatalf("after second human: humans/bots = %d/%d", humans, bots)
	}
	if teams != 44 {
		t.Fatalf("team sizes = %d, want 4/4", teams)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
