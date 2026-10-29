package game

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"killa/server/internal/protocol"
)

var (
	ErrRoomFull    = errors.New("room is full")
	ErrRoomClosed  = errors.New("room is closed")
	ErrJoinTimeout = errors.New("join timed out")
)

// Peer is the transport-facing side of a connected player.
type Peer interface {
	Send(v any) error
	Close() error
}

// Player is the authoritative state of one participant.
type Player struct {
	ID      string
	Name    string
	Team    int
	State   State
	HP      int
	Alive   bool
	Kills   int
	Deaths  int
	LastSeq uint32
	Peer    Peer
	IsBot   bool
	Brain   *botBrain

	Loadout     string
	Weapon      string
	Ammo        int
	Reloading   bool
	ReloadEnds  time.Time
	Vest        int
	Helmet      int
	Damagers    map[string]time.Time
	Assists     int
	LastDamaged time.Time

	LastShot       time.Time
	RespawnAt      time.Time
	ProtectedUntil time.Time
	RTT            time.Duration
	History        []PosSample
}

// PosSample is one entry in a player's position history, used for lag compensation.
type PosSample struct {
	T time.Time
	X float64
	Y float64
	Z float64
}

// Room owns all mutable game state and processes it on a single goroutine.
type Room struct {
	ID        string
	world     *World
	players   map[string]*Player
	byPeer    map[Peer]*Player
	inbox     chan any
	done      chan struct{}
	tickCount uint64

	matchState  string
	score       [2]int
	winner      int
	matchEndsAt time.Time
	stateEndsAt time.Time

	matchDuration time.Duration
	scoreLimit    int

	humans atomic.Int32
	total  atomic.Int32

	autoBots       bool
	botSeq         int
	nav            *navGrid
	nextBotBalance time.Time
}

type joinReq struct {
	peer   Peer
	name   string
	weapon string
	resp   chan error
}

type leaveReq struct{ peer Peer }

type inputReq struct {
	peer Peer
	in   protocol.Input
}

type shootReq struct {
	peer  Peer
	yaw   float64
	pitch float64
}

type loadoutReq struct {
	peer   Peer
	weapon string
}

type reloadReq struct{ peer Peer }

func NewRoom(id string, world *World) *Room {
	return &Room{
		ID:            id,
		world:         world,
		players:       make(map[string]*Player),
		byPeer:        make(map[Peer]*Player),
		inbox:         make(chan any, 512),
		done:          make(chan struct{}),
		matchState:    MatchWaiting,
		winner:        NoWinner,
		matchDuration: MatchDuration,
		scoreLimit:    ScoreLimit,
		nav:           buildNav(world),
	}
}

// Stats returns the current human and total player counts.
func (r *Room) Stats() (humans, total int) {
	return int(r.humans.Load()), int(r.total.Load())
}

// SetAutoBots controls whether the room fills empty slots with bots.
func (r *Room) SetAutoBots(enabled bool) {
	r.autoBots = enabled
}

// SetRules overrides match timing for development or custom rooms.
func (r *Room) SetRules(duration time.Duration, scoreLimit int) {
	if duration > 0 {
		r.matchDuration = duration
	}
	if scoreLimit > 0 {
		r.scoreLimit = scoreLimit
	}
}

// Run blocks until ctx is cancelled, simulating the room at TickRate.
func (r *Room) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second / TickRate)
	defer ticker.Stop()
	defer func() {
		r.closeAll()
		close(r.done)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-r.inbox:
			r.handle(msg)
		case <-ticker.C:
			r.tick()
		}
	}
}

func (r *Room) handle(msg any) {
	switch m := msg.(type) {
	case joinReq:
		r.join(m)
	case leaveReq:
		r.leave(m.peer)
	case inputReq:
		if p, ok := r.byPeer[m.peer]; ok {
			r.applyInput(p, m.in)
		}
	case shootReq:
		if p, ok := r.byPeer[m.peer]; ok {
			r.applyShot(p, m.yaw, m.pitch, time.Now())
		}
	case loadoutReq:
		if p, ok := r.byPeer[m.peer]; ok {
			if _, valid := weaponTable[m.weapon]; valid {
				p.Loadout = m.weapon
			}
		}
	case reloadReq:
		if p, ok := r.byPeer[m.peer]; ok {
			r.startReload(p, time.Now())
		}
	}
}

func (r *Room) join(m joinReq) {
	team := r.pickTeam()
	if r.teamSize(team) >= TeamSize {
		if !r.removeBotFrom(team) {
			other := 1 - team
			if r.teamSize(other) < TeamSize {
				team = other
			} else if r.removeBotFrom(other) {
				team = other
			} else {
				m.resp <- ErrRoomFull
				return
			}
		}
	}
	if len(r.players) >= MaxPlayers {
		m.resp <- ErrRoomFull
		return
	}

	loadout := DefaultWeapon
	if _, ok := weaponTable[m.weapon]; ok {
		loadout = m.weapon
	}

	spawn := r.spawnFor(team, r.teamSize(team))
	p := &Player{
		ID:      newID(),
		Name:    sanitizeName(m.name),
		Team:    team,
		HP:      MaxHealth,
		Alive:   true,
		Peer:    m.peer,
		State:   State{X: spawn.X, Y: 0, Z: spawn.Z, OnGround: true, Yaw: spawn.Yaw},
		History: make([]PosSample, 0, 32),
		Loadout: loadout,
	}
	r.equip(p)
	r.players[p.ID] = p
	r.byPeer[m.peer] = p
	r.humans.Add(1)
	r.total.Add(1)
	log.Printf("killa: %s joined %s team %d (%d/%d)", p.Name, r.ID, team, len(r.players), MaxPlayers)

	_ = m.peer.Send(protocol.Welcome{
		Type:     "welcome",
		PlayerID: p.ID,
		RoomID:   r.ID,
		Team:     team,
		TickRate: TickRate,
		Map:      r.world.ProtoMap(),
		Weapons:  weaponInfos(),
	})
	_ = m.peer.Send(r.snapshotFor(p, time.Now()))

	m.resp <- nil
}

func (r *Room) leave(peer Peer) {
	p, ok := r.byPeer[peer]
	if !ok {
		return
	}
	delete(r.byPeer, peer)
	delete(r.players, p.ID)
	r.total.Add(-1)
	if !p.IsBot {
		r.humans.Add(-1)
	}
	if !p.IsBot {
		log.Printf("killa: %s left %s (%d/%d)", p.Name, r.ID, len(r.players), MaxPlayers)
	}
	_ = p.Peer.Close()
}

func (r *Room) closeAll() {
	for peer, p := range r.byPeer {
		delete(r.byPeer, peer)
		delete(r.players, p.ID)
		_ = peer.Close()
	}
}

func (r *Room) tick() {
	now := time.Now()
	r.updateMatch(now)
	r.rebalanceBots(now)
	r.respawnDead(now)
	r.completeReloads(now)
	r.regen(now)
	r.updateBots(now, 1.0/float64(TickRate))
	r.tickCount++
	for _, p := range r.players {
		r.recordHistory(p, now)
		_ = p.Peer.Send(r.snapshotFor(p, now))
	}
}

func (r *Room) snapshotFor(me *Player, now time.Time) protocol.Snapshot {
	players := make([]protocol.Player, 0, len(r.players))
	for _, p := range r.players {
		respawnIn := 0.0
		if !p.Alive {
			if remaining := time.Until(p.RespawnAt); remaining > 0 {
				respawnIn = remaining.Seconds()
			}
		}
		reloadIn := 0.0
		if p.Reloading {
			if remaining := time.Until(p.ReloadEnds); remaining > 0 {
				reloadIn = remaining.Seconds()
			}
		}
		players = append(players, protocol.Player{
			ID:        p.ID,
			Name:      p.Name,
			Team:      p.Team,
			X:         p.State.X,
			Y:         p.State.Y,
			Z:         p.State.Z,
			Yaw:       p.State.Yaw,
			Pitch:     p.State.Pitch,
			HP:        p.HP,
			Alive:     p.Alive,
			Kills:     p.Kills,
			Deaths:    p.Deaths,
			RespawnIn: respawnIn,
			Weapon:    p.Weapon,
			Ammo:      p.Ammo,
			Reloading: p.Reloading,
			ReloadIn:  reloadIn,
			Vest:      p.Vest,
			Helmet:    p.Helmet,
			Ads:       p.State.Ads,
			Assists:   p.Assists,
		})
	}
	sort.Slice(players, func(i, j int) bool { return players[i].ID < players[j].ID })

	return protocol.Snapshot{
		Type:    "snapshot",
		Tick:    r.tickCount,
		Ack:     me.LastSeq,
		Match:   r.matchSnapshot(now),
		Players: players,
	}
}

func (r *Room) applyInput(p *Player, in protocol.Input) {
	if !finite(in.Dt) || !finite(in.Forward) || !finite(in.Right) || !finite(in.Yaw) || !finite(in.Pitch) {
		return
	}
	if p.LastSeq != 0 && in.Seq <= p.LastSeq {
		return
	}

	if finite(in.RTT) && in.RTT >= 0 {
		p.RTT = time.Duration(clamp(in.RTT, 0, 250) * float64(time.Millisecond))
	}

	in.Forward = clamp(in.Forward, -1, 1)
	in.Right = clamp(in.Right, -1, 1)
	p.LastSeq = in.Seq
	if !p.Alive {
		return
	}
	Step(&p.State, in, r.world)
}

// pickTeam prefers the team with fewer humans, then fewer total players.
func (r *Room) pickTeam() int {
	humans := [2]int{}
	totals := [2]int{}
	for _, p := range r.players {
		if p.Team != 0 && p.Team != 1 {
			continue
		}
		totals[p.Team]++
		if !p.IsBot {
			humans[p.Team]++
		}
	}
	if humans[0] != humans[1] {
		if humans[0] < humans[1] {
			return 0
		}
		return 1
	}
	if totals[0] != totals[1] {
		if totals[0] < totals[1] {
			return 0
		}
		return 1
	}
	return 0
}

func (r *Room) pickSpawn(team int) protocol.Spawn {
	used := 0
	for _, p := range r.players {
		if p.Team == team {
			used++
		}
	}
	return r.spawnFor(team, used)
}

// spawnFor returns the spawn point at index for a team, wrapping around.
func (r *Room) spawnFor(team, index int) protocol.Spawn {
	pool := make([]protocol.Spawn, 0, TeamSize)
	for _, s := range r.world.Spawns {
		if s.Team == team {
			pool = append(pool, s)
		}
	}
	if len(pool) == 0 {
		return protocol.Spawn{Team: team}
	}
	return pool[index%len(pool)]
}

// Join enqueues a join request and waits for the room to accept it.
func (r *Room) Join(peer Peer, name string, weapon ...string) error {
	resp := make(chan error, 1)
	chosen := ""
	if len(weapon) > 0 {
		chosen = weapon[0]
	}
	req := joinReq{peer: peer, name: name, weapon: chosen, resp: resp}

	select {
	case r.inbox <- req:
	case <-r.done:
		return ErrRoomClosed
	case <-time.After(5 * time.Second):
		return ErrJoinTimeout
	}

	select {
	case err := <-resp:
		return err
	case <-r.done:
		return ErrRoomClosed
	case <-time.After(5 * time.Second):
		return ErrJoinTimeout
	}
}

// Leave removes the peer from the room. It is safe to call more than once.
func (r *Room) Leave(peer Peer) {
	select {
	case r.inbox <- leaveReq{peer: peer}:
	case <-r.done:
	case <-time.After(time.Second):
	}
}

// SetLoadout stores the weapon the player will use on their next spawn.
func (r *Room) SetLoadout(peer Peer, weapon string) {
	select {
	case r.inbox <- loadoutReq{peer: peer, weapon: weapon}:
	case <-r.done:
	default:
	}
}

// Reload queues a reload request.
func (r *Room) Reload(peer Peer) {
	select {
	case r.inbox <- reloadReq{peer: peer}:
	case <-r.done:
	default:
	}
}

// Shoot queues a shot. The server decides whether it lands.
func (r *Room) Shoot(peer Peer, yaw, pitch float64) {
	select {
	case r.inbox <- shootReq{peer: peer, yaw: yaw, pitch: pitch}:
	case <-r.done:
	default:
	}
}

// Input queues an input frame. Under backpressure it is dropped rather than
// blocking the network read loop.
func (r *Room) Input(peer Peer, in protocol.Input) {
	select {
	case r.inbox <- inputReq{peer: peer, in: in}:
	case <-r.done:
	default:
	}
}

func sanitizeName(name string) string {
	var b strings.Builder
	count := 0
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsControl(r) {
			continue
		}
		if count >= MaxNameRunes {
			break
		}
		b.WriteRune(r)
		count++
	}
	if out := strings.TrimSpace(b.String()); out != "" {
		return out
	}
	return "Player"
}

func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

// equip gives the player their selected loadout, full ammo and fresh armor.
func (r *Room) equip(p *Player) {
	if _, ok := weaponTable[p.Loadout]; !ok {
		p.Loadout = DefaultWeapon
	}
	p.Weapon = p.Loadout
	w := weaponByID(p.Weapon)
	p.Ammo = w.MagSize
	p.Reloading = false
	p.ReloadEnds = time.Time{}
	p.Vest = ArmorDurability
	p.Helmet = ArmorDurability
}

func (r *Room) startReload(p *Player, now time.Time) {
	if !p.Alive || p.Reloading {
		return
	}
	w := weaponByID(p.Weapon)
	if p.Ammo >= w.MagSize {
		return
	}
	p.Reloading = true
	p.ReloadEnds = now.Add(w.ReloadTime)
}

// regen restores health after a few seconds without taking damage.
func (r *Room) regen(now time.Time) {
	for _, p := range r.players {
		if !p.Alive || p.HP >= MaxHealth || p.LastDamaged.IsZero() {
			continue
		}
		if now.Sub(p.LastDamaged) < RegenDelay {
			continue
		}
		p.HP++
		if p.HP > MaxHealth {
			p.HP = MaxHealth
		}
	}
}

func (r *Room) completeReloads(now time.Time) {
	for _, p := range r.players {
		if !p.Alive || !p.Reloading || now.Before(p.ReloadEnds) {
			continue
		}
		p.Ammo = weaponByID(p.Weapon).MagSize
		p.Reloading = false
		p.ReloadEnds = time.Time{}
	}
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
