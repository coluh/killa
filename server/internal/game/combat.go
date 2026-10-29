package game

import (
	"math"
	"math/rand"
	"time"

	"killa/server/internal/protocol"
)

// rayCylinder intersects a vertical cylinder (base y=0) with a ray.
// Returns the hit distance, whether the ray hit, and whether the hit was in the
// head region.
func rayCylinder(
	ox, oy, oz, dx, dy, dz float64,
	baseY, cx, cz, radius, height, headHeight float64,
) (float64, bool, bool) {
	ax := ox - cx
	az := oz - cz

	a := dx*dx + dz*dz
	b := 2 * (ax*dx + az*dz)
	c := ax*ax + az*az - radius*radius

	var t float64
	if a < 1e-9 {
		if c > 0 {
			return 0, false, false
		}
		t = 0
	} else {
		disc := b*b - 4*a*c
		if disc < 0 {
			return 0, false, false
		}
		root := math.Sqrt(disc)
		t0 := (-b - root) / (2 * a)
		t1 := (-b + root) / (2 * a)
		switch {
		case t0 >= 0:
			t = t0
		case t1 >= 0:
			t = t1
		default:
			return 0, false, false
		}
	}

	y := oy + t*dy
	if y < baseY || y > baseY+height {
		return 0, false, false
	}
	return t, true, y >= baseY+headHeight
}

// rayAABB returns the entry distance of a ray into an axis-aligned box.
func rayAABB(
	ox, oy, oz, dx, dy, dz float64,
	minX, minY, minZ, maxX, maxY, maxZ float64,
) (float64, bool) {
	origins := [3]float64{ox, oy, oz}
	dirs := [3]float64{dx, dy, dz}
	mins := [3]float64{minX, minY, minZ}
	maxs := [3]float64{maxX, maxY, maxZ}

	tmin := 0.0
	tmax := math.Inf(1)

	for i := 0; i < 3; i++ {
		o := origins[i]
		d := dirs[i]
		if math.Abs(d) < 1e-9 {
			if o < mins[i] || o > maxs[i] {
				return 0, false
			}
			continue
		}
		t1 := (mins[i] - o) / d
		t2 := (maxs[i] - o) / d
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tmin {
			tmin = t1
		}
		if t2 < tmax {
			tmax = t2
		}
		if tmin > tmax {
			return 0, false
		}
	}
	return tmin, true
}

// applyShot performs an authoritative hitscan for one shot.
func (r *Room) applyShot(p *Player, yaw, pitch float64, now time.Time) {
	if !p.Alive || !finite(yaw) || !finite(pitch) {
		return
	}
	weapon := weaponByID(p.Weapon)
	if p.Reloading || now.Sub(p.LastShot) < weapon.FireInterval {
		return
	}
	if p.Ammo <= 0 {
		r.startReload(p, now)
		return
	}
	p.LastShot = now
	p.Ammo--
	if p.Ammo == 0 {
		r.startReload(p, now)
	}

	pitch = clamp(pitch, -PitchLimit, PitchLimit)
	spread := weapon.Spread
	if p.State.Ads {
		spread *= AdsSpreadMultiplier
	}
	yaw += (rand.Float64()*2 - 1) * spread
	pitch = clamp(pitch+(rand.Float64()*2-1)*spread*0.6, -PitchLimit, PitchLimit)

	ox := p.State.X
	oy := p.State.Y + EyeHeight
	oz := p.State.Z

	cosPitch := math.Cos(pitch)
	dx := -math.Sin(yaw) * cosPitch
	dy := math.Sin(pitch)
	dz := -math.Cos(yaw) * cosPitch

	maxDist := weapon.Range
	for _, c := range r.world.Crates {
		t, ok := rayAABB(ox, oy, oz, dx, dy, dz,
			c.X-c.W/2, 0, c.Z-c.D/2,
			c.X+c.W/2, c.H, c.Z+c.D/2)
		if ok && t < maxDist {
			maxDist = t
		}
	}

	aimTime := now.Add(-p.RTT / 2)
	var best *Player
	bestT := maxDist
	headshot := false

	for _, target := range r.players {
		if target.ID == p.ID || target.Team == p.Team || !target.Alive {
			continue
		}
		tx, ty, tz := target.positionAt(aimTime)
		t, hit, head := rayCylinder(ox, oy, oz, dx, dy, dz, ty, tx, tz, PlayerRadius, PlayerHeight, HeadHeight)
		if !hit || t >= bestT {
			continue
		}
		best = target
		bestT = t
		headshot = head
	}

	hitID := ""
	endT := bestT
	if best != nil && !now.Before(best.ProtectedUntil) {
		hitID = best.ID
		damage := weapon.Damage
		if headshot {
			damage = weapon.HeadDamage
		}
		if best.Damagers == nil {
			best.Damagers = make(map[string]time.Time)
		}
		best.Damagers[p.ID] = now
		damage = applyArmor(best, damage, headshot)
		best.HP -= damage
		if best.HP <= 0 {
			best.HP = 0
			r.kill(p, best, now)
		}
	}

	r.broadcastEvent(protocol.Shot{
		Type:     "shot",
		Shooter:  p.ID,
		X:        ox,
		Y:        oy,
		Z:        oz,
		EndX:     ox + dx*endT,
		EndY:     oy + dy*endT,
		EndZ:     oz + dz*endT,
		Hit:      hitID,
		Headshot: hitID != "" && headshot,
	})
}

// applyArmor soaks part of the damage with the vest or helmet.
func applyArmor(target *Player, damage int, headshot bool) int {
	if headshot {
		if target.Helmet <= 0 {
			return damage
		}
		absorbed := damage * 35 / 100
		if absorbed > target.Helmet {
			absorbed = target.Helmet
		}
		target.Helmet -= absorbed
		return damage - absorbed
	}

	if target.Vest <= 0 {
		return damage
	}
	absorbed := damage * 35 / 100
	if absorbed > target.Vest {
		absorbed = target.Vest
	}
	target.Vest -= absorbed
	return damage - absorbed
}

func (r *Room) kill(killer, victim *Player, now time.Time) {
	victim.Alive = false
	victim.HP = 0
	victim.Deaths++
	victim.RespawnAt = now.Add(RespawnDelay)
	killer.Kills++
	for id, damagedAt := range victim.Damagers {
		if id == killer.ID || now.Sub(damagedAt) > AssistWindow {
			continue
		}
		if helper := r.players[id]; helper != nil {
			helper.Assists++
		}
	}
	victim.Damagers = nil

	if r.matchState == MatchLive {
		r.score[killer.Team]++
		if r.score[killer.Team] >= r.scoreLimit {
			r.finishMatch(now)
		}
	}

	r.broadcastEvent(protocol.Kill{
		Type:       "kill",
		Killer:     killer.ID,
		KillerName: killer.Name,
		Victim:     victim.ID,
		VictimName: victim.Name,
	})
}

func (r *Room) respawnDead(now time.Time) {
	for _, p := range r.players {
		if p.Alive || p.RespawnAt.IsZero() || now.Before(p.RespawnAt) {
			continue
		}
		spawn := r.pickSpawn(p.Team)
		p.State = State{X: spawn.X, Z: spawn.Z, OnGround: true, Yaw: spawn.Yaw}
		p.HP = MaxHealth
		p.Alive = true
		p.ProtectedUntil = now.Add(SpawnProtectionTime)
		p.History = p.History[:0]
		p.LastShot = time.Time{}
		p.Damagers = nil
		r.equip(p)
		if p.Brain != nil {
			p.Brain.targetID = ""
			p.Brain.path = nil
			p.Brain.nextThink = now
			p.Brain.nextPath = now
			p.Brain.burst = 0
			p.Brain.burstPause = time.Time{}
		}
	}
}

// lineOfSight reports whether the segment is clear of crates.
func (w *World) lineOfSight(ax, ay, az, bx, by, bz float64) bool {
	dx := bx - ax
	dy := by - ay
	dz := bz - az
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist < 1e-6 {
		return true
	}
	dx /= dist
	dy /= dist
	dz /= dist

	for _, c := range w.Crates {
		t, ok := rayAABB(ax, ay, az, dx, dy, dz,
			c.X-c.W/2, 0, c.Z-c.D/2,
			c.X+c.W/2, c.H, c.Z+c.D/2)
		if ok && t < dist-0.05 {
			return false
		}
	}
	return true
}

// positionAt returns the interpolated position at t from the recorded history.
func (p *Player) positionAt(t time.Time) (float64, float64, float64) {
	h := p.History
	if len(h) == 0 {
		return p.State.X, p.State.Y, p.State.Z
	}
	if !t.After(h[0].T) {
		return h[0].X, h[0].Y, h[0].Z
	}
	last := h[len(h)-1]
	if !t.Before(last.T) {
		return last.X, last.Y, last.Z
	}

	for i := 1; i < len(h); i++ {
		if t.After(h[i].T) {
			continue
		}
		prev := h[i-1]
		next := h[i]
		span := next.T.Sub(prev.T).Seconds()
		if span <= 0 {
			return next.X, next.Y, next.Z
		}
		alpha := t.Sub(prev.T).Seconds() / span
		return prev.X + (next.X-prev.X)*alpha,
			prev.Y + (next.Y-prev.Y)*alpha,
			prev.Z + (next.Z-prev.Z)*alpha
	}
	return last.X, last.Y, last.Z
}

func (r *Room) recordHistory(p *Player, now time.Time) {
	p.History = append(p.History, PosSample{
		T: now,
		X: p.State.X,
		Y: p.State.Y,
		Z: p.State.Z,
	})
	if len(p.History) > 32 {
		p.History = p.History[len(p.History)-32:]
	}
}

func (r *Room) broadcastEvent(v any) {
	for _, p := range r.players {
		_ = p.Peer.Send(v)
	}
}
