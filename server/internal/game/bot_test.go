package game

import (
	"math"
	"testing"
	"time"

	"killa/server/internal/protocol"
)

func newBotRoom(crates []protocol.Crate) *Room {
	world := &World{
		Bounds: protocol.Bounds{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
		Crates: crates,
		Spawns: []protocol.Spawn{
			{Team: 0, X: -2, Z: 16, Yaw: 0},
			{Team: 0, X: 2, Z: 16, Yaw: 0},
			{Team: 0, X: -5, Z: 16, Yaw: 0},
			{Team: 0, X: 5, Z: 16, Yaw: 0},
			{Team: 1, X: -2, Z: -16, Yaw: math.Pi},
			{Team: 1, X: 2, Z: -16, Yaw: math.Pi},
			{Team: 1, X: -5, Z: -16, Yaw: math.Pi},
			{Team: 1, X: 5, Z: -16, Yaw: math.Pi},
		},
	}
	return NewRoom("bots", world)
}

func TestRebalanceFillsTeams(t *testing.T) {
	room := newBotRoom(nil)
	room.SetAutoBots(true)

	human := &Player{ID: "human", Name: "human", Team: 0, HP: MaxHealth, Alive: true, Peer: &fakePeer{}}
	room.players[human.ID] = human
	room.byPeer[human.Peer] = human

	now := time.Unix(1000, 0)
	room.rebalanceBots(now)

	if len(room.players) != MaxPlayers {
		t.Fatalf("players = %d, want %d", len(room.players), MaxPlayers)
	}
	if room.botCount() != MaxPlayers-1 {
		t.Fatalf("bots = %d, want %d", room.botCount(), MaxPlayers-1)
	}
	if room.teamSize(0) != TeamSize || room.teamSize(1) != TeamSize {
		t.Fatalf("team sizes = %d/%d, want 4/4", room.teamSize(0), room.teamSize(1))
	}

	room.removePlayer(human)
	room.nextBotBalance = time.Time{}
	room.rebalanceBots(now.Add(time.Second))

	if len(room.players) != 0 {
		t.Fatalf("bots were not cleared after last human left: %d", len(room.players))
	}
}

func TestBotShootsEnemy(t *testing.T) {
	room := newBotRoom(nil)
	now := time.Unix(1000, 0)

	bot := room.addBot(0, now)
	bot.State = State{X: 0, Z: 5, Yaw: 0}
	bot.Brain.reaction = 0

	enemy := &Player{
		ID: "enemy", Name: "enemy", Team: 1, HP: MaxHealth, Alive: true,
		Peer: &fakePeer{}, State: State{X: 0, Z: -5, Yaw: math.Pi},
	}
	room.players[enemy.ID] = enemy
	room.byPeer[enemy.Peer] = enemy

	for i := 0; i < 120; i++ {
		ts := now.Add(time.Duration(i) * 50 * time.Millisecond)
		room.updateBots(ts, 0.05)
		if !enemy.Alive || enemy.HP < MaxHealth {
			break
		}
	}

	if enemy.Alive && enemy.HP == MaxHealth {
		t.Fatal("bot never hit the enemy")
	}
}

func TestBotNavigatesAroundWall(t *testing.T) {
	room := newBotRoom([]protocol.Crate{{X: 0, Z: 0, W: 10, D: 2, H: 2}})
	now := time.Unix(1000, 0)

	bot := room.addBot(0, now)
	bot.State = State{X: 0, Z: 6, Yaw: 0}

	enemy := &Player{
		ID: "enemy", Name: "enemy", Team: 1, HP: MaxHealth, Alive: true,
		Peer: &fakePeer{}, State: State{X: 0, Z: -6, Yaw: math.Pi},
	}
	room.players[enemy.ID] = enemy
	room.byPeer[enemy.Peer] = enemy

	start := math.Hypot(bot.State.X-enemy.State.X, bot.State.Z-enemy.State.Z)

	for i := 0; i < 160; i++ {
		ts := now.Add(time.Duration(i) * 50 * time.Millisecond)
		room.updateBots(ts, 0.05)
		if !enemy.Alive {
			break
		}
	}

	dist := math.Hypot(bot.State.X-enemy.State.X, bot.State.Z-enemy.State.Z)
	if dist > start-3 {
		t.Fatalf("bot did not approach the enemy: start=%v now=%v", start, dist)
	}
}

func TestBotTeamAssignment(t *testing.T) {
	room := newBotRoom(nil)
	room.SetAutoBots(true)

	human := &Player{ID: "h", Name: "h", Team: 1, HP: MaxHealth, Alive: true, Peer: &fakePeer{}}
	room.players[human.ID] = human
	room.byPeer[human.Peer] = human

	room.rebalanceBots(time.Unix(1000, 0))

	if room.teamSize(1) != TeamSize {
		t.Fatalf("human team size = %d, want %d", room.teamSize(1), TeamSize)
	}
	if room.teamSize(0) != TeamSize {
		t.Fatalf("other team size = %d, want %d", room.teamSize(0), TeamSize)
	}
}
