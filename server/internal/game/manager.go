package game

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Manager owns a pool of rooms and groups players into them.
type Manager struct {
	ctx      context.Context
	world    *World
	maxRooms int

	mu    sync.Mutex
	rooms map[string]*Room
	seq   int

	autoBots      bool
	matchDuration time.Duration
	scoreLimit    int
	matchRecorder func(MatchResult)
}

func NewManager(ctx context.Context, world *World, maxRooms int) *Manager {
	if maxRooms < 1 {
		maxRooms = 1
	}
	return &Manager{
		ctx:           ctx,
		world:         world,
		maxRooms:      maxRooms,
		rooms:         make(map[string]*Room),
		autoBots:      true,
		matchDuration: MatchDuration,
		scoreLimit:    ScoreLimit,
	}
}

func (m *Manager) SetAutoBots(enabled bool) {
	m.mu.Lock()
	m.autoBots = enabled
	m.mu.Unlock()
}

func (m *Manager) SetRules(duration time.Duration, scoreLimit int) {
	m.mu.Lock()
	if duration > 0 {
		m.matchDuration = duration
	}
	if scoreLimit > 0 {
		m.scoreLimit = scoreLimit
	}
	m.mu.Unlock()
}

// Join places the peer in the fullest room that still has human capacity,
// creating a room when needed. ErrRoomFull means the whole process is full.
func (m *Manager) Join(peer Peer, name string, weapon ...string) (*Room, error) {
	return m.JoinProfile(peer, name, firstOrEmpty(weapon...), "")
}

// JoinProfile places a session-backed player into a room.
func (m *Manager) JoinProfile(peer Peer, name, weapon, profileID string) (*Room, error) {
	for attempt := 0; attempt < 6; attempt++ {
		m.mu.Lock()
		room := m.pickRoomLocked()
		m.mu.Unlock()

		if room == nil {
			return nil, ErrRoomFull
		}

		err := room.JoinProfile(peer, name, weapon, profileID)
		if err == nil {
			return room, nil
		}
		if !errors.Is(err, ErrRoomFull) {
			return nil, err
		}
	}
	return nil, ErrRoomFull
}

// SetMatchRecorder receives finished matches from all rooms.
func (m *Manager) SetMatchRecorder(fn func(MatchResult)) {
	m.mu.Lock()
	m.matchRecorder = fn
	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	m.mu.Unlock()
	for _, room := range rooms {
		room.SetMatchRecorder(fn)
	}
}

func (m *Manager) pickRoomLocked() *Room {
	var best *Room
	bestHumans := -1
	var empty *Room

	for _, room := range m.rooms {
		humans, _ := room.Stats()
		if humans >= MaxPlayers {
			continue
		}
		if humans == 0 {
			if empty == nil {
				empty = room
			}
			continue
		}
		if humans > bestHumans {
			best = room
			bestHumans = humans
		}
	}

	if best != nil {
		return best
	}
	if empty != nil {
		return empty
	}
	if len(m.rooms) < m.maxRooms {
		return m.createRoomLocked()
	}
	return nil
}

func (m *Manager) createRoomLocked() *Room {
	m.seq++
	room := NewRoom(fmt.Sprintf("arena-%d", m.seq), m.world)
	room.SetAutoBots(m.autoBots)
	room.SetRules(m.matchDuration, m.scoreLimit)
	room.SetMatchRecorder(m.matchRecorder)
	m.rooms[room.ID] = room
	go room.Run(m.ctx)
	return room
}

// Status is a lightweight aggregate for ops endpoints.
type Status struct {
	Rooms   int
	Humans  int
	Players int
}

// Status snapshots room and player counts across the process.
func (m *Manager) Status() Status {
	m.mu.Lock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	m.mu.Unlock()

	st := Status{Rooms: len(rooms)}
	for _, room := range rooms {
		humans, total := room.Stats()
		st.Humans += humans
		st.Players += total
	}
	return st
}

// RoomCount reports how many rooms currently exist.
func (m *Manager) RoomCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rooms)
}
