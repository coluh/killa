package game

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// botPeer is a sink for events addressed to a bot.
type botPeer struct{}

func (*botPeer) Send(any) error { return nil }
func (*botPeer) Close() error   { return nil }

type botBrain struct {
	targetID string

	path    []Vec2
	pathIdx int

	nextThink     time.Time
	nextPath      time.Time
	nextShot      time.Time
	reactionUntil time.Time
	burst         int
	burstPause    time.Time

	aimErr   float64
	turnRate float64
	reaction time.Duration

	strafeDir   float64
	strafeUntil time.Time

	lastX, lastZ   float64
	nextStuckCheck time.Time
}

func newBotBrain(now time.Time) *botBrain {
	return &botBrain{
		aimErr:         0.05 + rand.Float64()*0.07,
		turnRate:       1.5 + rand.Float64()*1.3,
		reaction:       time.Duration(600+rand.Intn(600)) * time.Millisecond,
		nextThink:      now,
		nextStuckCheck: now.Add(time.Second),
	}
}

func (r *Room) addBot(team int, now time.Time) *Player {
	r.botSeq++
	spawn := r.spawnFor(team, r.teamSize(team))
	loadout := "ar"
	if rand.Intn(3) == 0 {
		loadout = "smg"
	}
	p := &Player{
		ID:             "bot-" + newID(),
		Name:           fmt.Sprintf("BOT-%02d", r.botSeq),
		Team:           team,
		HP:             MaxHealth,
		Alive:          true,
		IsBot:          true,
		Peer:           new(botPeer),
		State:          State{X: spawn.X, Z: spawn.Z, OnGround: true, Yaw: spawn.Yaw},
		ProtectedUntil: now.Add(SpawnProtectionTime),
		History:        make([]PosSample, 0, 32),
		Brain:          newBotBrain(now),
		Loadout:        loadout,
	}
	r.equip(p)
	r.players[p.ID] = p
	r.byPeer[p.Peer] = p
	r.total.Add(1)
	return p
}

func (r *Room) removeBotFrom(team int) bool {
	for _, p := range r.players {
		if p.IsBot && p.Team == team {
			r.removePlayer(p)
			return true
		}
	}
	return false
}

func (r *Room) removeAnyBot() bool {
	best := -1
	bestSize := -1
	for team := 0; team < 2; team++ {
		size := r.teamSize(team)
		if size > bestSize && r.hasBot(team) {
			best = team
			bestSize = size
		}
	}
	if best < 0 {
		return false
	}
	return r.removeBotFrom(best)
}

func (r *Room) hasBot(team int) bool {
	for _, p := range r.players {
		if p.IsBot && p.Team == team {
			return true
		}
	}
	return false
}

func (r *Room) removePlayer(p *Player) {
	delete(r.players, p.ID)
	delete(r.byPeer, p.Peer)
	if p.IsBot {
		r.total.Add(-1)
	} else {
		r.humans.Add(-1)
		r.total.Add(-1)
	}
	_ = p.Peer.Close()
}

func (r *Room) teamSize(team int) int {
	size := 0
	for _, p := range r.players {
		if p.Team == team {
			size++
		}
	}
	return size
}

func (r *Room) humanCount() int {
	humans := 0
	for _, p := range r.players {
		if !p.IsBot {
			humans++
		}
	}
	return humans
}

func (r *Room) botCount() int {
	bots := 0
	for _, p := range r.players {
		if p.IsBot {
			bots++
		}
	}
	return bots
}

// rebalanceBots keeps the room near 8 players by filling/removing bots.
func (r *Room) rebalanceBots(now time.Time) {
	if !r.autoBots {
		return
	}
	if now.Before(r.nextBotBalance) {
		return
	}
	r.nextBotBalance = now.Add(500 * time.Millisecond)

	humans := r.humanCount()
	if humans == 0 {
		for r.botCount() > 0 {
			if !r.removeAnyBot() {
				return
			}
		}
		return
	}

	desired := MaxPlayers - humans
	for r.botCount() > desired {
		if !r.removeAnyBot() {
			return
		}
	}
	for r.botCount() < desired {
		team := r.botTeamForNew()
		if team < 0 {
			return
		}
		r.addBot(team, now)
	}
}

func (r *Room) botTeamForNew() int {
	size0 := r.teamSize(0)
	size1 := r.teamSize(1)

	preferred := 0
	if size1 < size0 {
		preferred = 1
	}
	if r.teamSize(preferred) < TeamSize {
		return preferred
	}
	other := 1 - preferred
	if r.teamSize(other) < TeamSize {
		return other
	}
	return -1
}

func (r *Room) updateBots(now time.Time, dt float64) {
	for _, p := range r.players {
		if !p.IsBot || !p.Alive || p.Brain == nil {
			continue
		}
		r.updateBot(p, now, dt)
	}
}

func (r *Room) updateBot(p *Player, now time.Time, dt float64) {
	if r.invincible {
		return
	}
	b := p.Brain

	if now.After(b.nextThink) {
		b.nextThink = now.Add(400*time.Millisecond + time.Duration(rand.Intn(200))*time.Millisecond)
		target := r.pickBotTarget(p)
		if target == nil {
			b.targetID = ""
		} else if target.ID != b.targetID {
			b.targetID = target.ID
			b.reactionUntil = now.Add(b.reaction + time.Duration(rand.Intn(150))*time.Millisecond)
			b.path = nil
			b.nextPath = now
		}
	}

	var target *Player
	if b.targetID != "" {
		target = r.players[b.targetID]
	}
	if target != nil && (!target.Alive || target.Team == p.Team) {
		target = nil
		b.targetID = ""
	}

	if target == nil {
		r.botRoam(p, b, now, dt)
		return
	}

	eyeY := p.State.Y + EyeHeight
	aimX := target.State.X
	aimY := target.State.Y + 1.1
	aimZ := target.State.Z
	dist := math.Hypot(aimX-p.State.X, aimZ-p.State.Z)
	los := r.world.lineOfSight(p.State.X, eyeY, p.State.Z, aimX, aimY, aimZ)

	if los && dist <= 20 {
		yaw, pitch := aimAngles(p.State.X, eyeY, p.State.Z, aimX, aimY, aimZ)
		r.turnBot(p, b, yaw, pitch, dt)
		if dist < 3.5 {
			// Keep some distance instead of standing inside the target.
			r.moveBot(p, p.State.X-aimX, p.State.Z-aimZ, WalkSpeed, dt)
		} else {
			r.botStrafe(p, b, now, dt, aimX, aimZ)
		}

		aligned := math.Abs(shortestAngle(p.State.Yaw, yaw)) < 0.045 &&
			math.Abs(p.State.Pitch-pitch) < 0.04
		if aligned && now.After(b.reactionUntil) && now.After(b.burstPause) && now.After(b.nextShot) {
			errYaw := (rand.Float64()*2 - 1) * b.aimErr
			errPitch := (rand.Float64()*2 - 1) * b.aimErr * 0.6
			r.applyShot(p, p.State.Yaw+errYaw, p.State.Pitch+errPitch, now)

			b.nextShot = now.Add(320*time.Millisecond + time.Duration(rand.Intn(160))*time.Millisecond)
			b.burst++
			if b.burst >= 2+rand.Intn(2) {
				b.burst = 0
				b.burstPause = now.Add(time.Duration(600+rand.Intn(600)) * time.Millisecond)
			}
		}
		return
	}

	r.botMoveTo(p, b, now, dt, aimX, aimZ)
}

func (r *Room) botRoam(p *Player, b *botBrain, now time.Time, dt float64) {
	goalZ := -8.0
	if p.Team == 1 {
		goalZ = 8.0
	}
	r.botMoveTo(p, b, now, dt, 0, goalZ)
}

func (r *Room) botMoveTo(p *Player, b *botBrain, now time.Time, dt, goalX, goalZ float64) {
	if now.After(b.nextPath) || len(b.path) == 0 {
		b.path = r.nav.findPath(Vec2{X: p.State.X, Z: p.State.Z}, Vec2{X: goalX, Z: goalZ})
		b.pathIdx = 0
		b.nextPath = now.Add(900*time.Millisecond + time.Duration(rand.Intn(400))*time.Millisecond)
	}

	if now.After(b.nextStuckCheck) {
		moved := math.Hypot(p.State.X-b.lastX, p.State.Z-b.lastZ)
		if moved < 0.15 {
			b.path = nil
			b.nextPath = now
		}
		b.lastX, b.lastZ = p.State.X, p.State.Z
		b.nextStuckCheck = now.Add(time.Second)
	}

	if len(b.path) == 0 {
		dx := goalX - p.State.X
		dz := goalZ - p.State.Z
		r.moveBot(p, dx, dz, WalkSpeed, dt)
		r.faceDirection(p, b, dx, dz, dt)
		return
	}

	for b.pathIdx < len(b.path)-1 {
		wp := b.path[b.pathIdx]
		if math.Hypot(wp.X-p.State.X, wp.Z-p.State.Z) < 0.7 {
			b.pathIdx++
		} else {
			break
		}
	}

	wp := b.path[b.pathIdx]
	dx := wp.X - p.State.X
	dz := wp.Z - p.State.Z
	speed := WalkSpeed
	if math.Hypot(goalX-p.State.X, goalZ-p.State.Z) > 12 {
		speed = SprintSpeed
	}
	r.moveBot(p, dx, dz, speed, dt)
	r.faceDirection(p, b, dx, dz, dt)
}

func (r *Room) moveBot(p *Player, dx, dz, speed, dt float64) {
	// Separation from other players so bots do not stand inside someone.
	for _, q := range r.players {
		if q.ID == p.ID || !q.Alive {
			continue
		}
		ox := p.State.X - q.State.X
		oz := p.State.Z - q.State.Z
		dist := math.Hypot(ox, oz)
		if dist > 1e-4 && dist < 1.2 {
			push := (1.2 - dist) * 0.9
			dx += ox / dist * push
			dz += oz / dist * push
		}
	}

	mag := math.Hypot(dx, dz)
	if mag < 1e-6 {
		return
	}
	dx /= mag
	dz /= mag

	if nx := p.State.X + dx*speed*dt; !r.world.blocked(nx, p.State.Z, PlayerRadius) {
		p.State.X = nx
	}
	if nz := p.State.Z + dz*speed*dt; !r.world.blocked(p.State.X, nz, PlayerRadius) {
		p.State.Z = nz
	}
	p.State.X, p.State.Z = r.world.withinBounds(p.State.X, p.State.Z, PlayerRadius)
}

func (r *Room) faceDirection(p *Player, b *botBrain, dx, dz, dt float64) {
	if math.Hypot(dx, dz) < 1e-6 {
		return
	}
	yaw := math.Atan2(-dx, -dz)
	r.turnBot(p, b, yaw, 0, dt)
}

func (r *Room) turnBot(p *Player, b *botBrain, yaw, pitch, dt float64) {
	maxTurn := b.turnRate * dt
	delta := shortestAngle(p.State.Yaw, yaw)
	if math.Abs(delta) <= maxTurn {
		p.State.Yaw = yaw
	} else if delta > 0 {
		p.State.Yaw += maxTurn
	} else {
		p.State.Yaw -= maxTurn
	}

	maxPitch := maxTurn * 0.6
	dp := pitch - p.State.Pitch
	if math.Abs(dp) <= maxPitch {
		p.State.Pitch = pitch
	} else if dp > 0 {
		p.State.Pitch += maxPitch
	} else {
		p.State.Pitch -= maxPitch
	}
	p.State.Pitch = clamp(p.State.Pitch, -PitchLimit, PitchLimit)
}

func (r *Room) botStrafe(p *Player, b *botBrain, now time.Time, dt, targetX, targetZ float64) {
	if now.After(b.strafeUntil) {
		b.strafeUntil = now.Add(time.Duration(700+rand.Intn(700)) * time.Millisecond)
		if rand.Intn(2) == 0 {
			b.strafeDir = -1
		} else {
			b.strafeDir = 1
		}
	}
	dx := targetX - p.State.X
	dz := targetZ - p.State.Z
	r.moveBot(p, -dz*b.strafeDir, dx*b.strafeDir, 1.2, dt)
}

func (r *Room) pickBotTarget(p *Player) *Player {
	var best *Player
	bestScore := math.Inf(1)

	for _, q := range r.players {
		if q.ID == p.ID || q.Team == p.Team || !q.Alive {
			continue
		}
		dist := math.Hypot(q.State.X-p.State.X, q.State.Z-p.State.Z)
		score := dist
		if !r.world.lineOfSight(p.State.X, p.State.Y+EyeHeight, p.State.Z,
			q.State.X, q.State.Y+1.1, q.State.Z) {
			score += 120
		}
		if score < bestScore {
			bestScore = score
			best = q
		}
	}
	return best
}

func aimAngles(fromX, fromY, fromZ, toX, toY, toZ float64) (float64, float64) {
	dx := toX - fromX
	dy := toY - fromY
	dz := toZ - fromZ
	horizontal := math.Hypot(dx, dz)
	yaw := math.Atan2(-dx, -dz)
	pitch := math.Atan2(dy, horizontal)
	return yaw, pitch
}

func shortestAngle(from, to float64) float64 {
	delta := math.Mod(to-from+math.Pi, 2*math.Pi)
	if delta < 0 {
		delta += 2 * math.Pi
	}
	return delta - math.Pi
}
