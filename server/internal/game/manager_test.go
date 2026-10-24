package game

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestManagerGroupsPlayersIntoRooms(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManager(ctx, NewWarehouse(), 2)
	manager.SetAutoBots(true)

	rooms := make([]*Room, 0, 9)
	for i := 0; i < 9; i++ {
		room, err := manager.Join(&fakePeer{}, fmt.Sprintf("h%d", i))
		if err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		rooms = append(rooms, room)
	}

	for i := 0; i < 8; i++ {
		if rooms[i].ID != rooms[0].ID {
			t.Fatalf("player %d landed in %s, want %s", i, rooms[i].ID, rooms[0].ID)
		}
	}
	if rooms[8].ID == rooms[0].ID {
		t.Fatal("ninth player should create a second room")
	}
	if manager.RoomCount() != 2 {
		t.Fatalf("rooms = %d, want 2", manager.RoomCount())
	}
	if humans, _ := rooms[0].Stats(); humans != MaxPlayers {
		t.Fatalf("room 1 humans = %d, want %d", humans, MaxPlayers)
	}

	for i := 9; i < 16; i++ {
		room, err := manager.Join(&fakePeer{}, fmt.Sprintf("h%d", i))
		if err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		if room.ID != rooms[8].ID {
			t.Fatalf("player %d landed in %s, want %s", i, room.ID, rooms[8].ID)
		}
	}
	if humans, _ := rooms[8].Stats(); humans != MaxPlayers {
		t.Fatalf("room 2 humans = %d, want %d", humans, MaxPlayers)
	}

	if _, err := manager.Join(&fakePeer{}, "overflow"); !errors.Is(err, ErrRoomFull) {
		t.Fatalf("overflow error = %v, want ErrRoomFull", err)
	}
}

func TestManagerStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManager(ctx, NewWarehouse(), 2)
	manager.SetAutoBots(true)
	for i := 0; i < 9; i++ {
		if _, err := manager.Join(&fakePeer{}, fmt.Sprintf("s%d", i)); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
	}

	status := manager.Status()
	if status.Rooms != 2 {
		t.Fatalf("rooms = %d, want 2", status.Rooms)
	}
	if status.Humans != 9 {
		t.Fatalf("humans = %d, want 9", status.Humans)
	}
}

func TestManagerReusesEmptyRoom(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewManager(ctx, NewWarehouse(), 3)
	manager.SetAutoBots(false)

	peers := make([]*fakePeer, 0, 10)
	rooms := make([]*Room, 0, 10)
	for i := 0; i < 9; i++ {
		peer := &fakePeer{}
		room, err := manager.Join(peer, fmt.Sprintf("r%d", i))
		if err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
		peers = append(peers, peer)
		rooms = append(rooms, room)
	}
	second := rooms[8]
	if manager.RoomCount() != 2 {
		t.Fatalf("rooms = %d, want 2", manager.RoomCount())
	}

	second.Leave(peers[8])
	deadline := time.Now().Add(time.Second)
	for {
		if humans, _ := second.Stats(); humans == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("room did not empty")
		}
		time.Sleep(10 * time.Millisecond)
	}

	third, err := manager.Join(&fakePeer{}, "again")
	if err != nil {
		t.Fatalf("join after leave: %v", err)
	}
	if third.ID != second.ID {
		t.Fatalf("joined %s, want empty room %s", third.ID, second.ID)
	}
	if manager.RoomCount() != 2 {
		t.Fatalf("rooms = %d, want reused 2", manager.RoomCount())
	}
}
