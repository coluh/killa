package game

import (
	"log"
	"math"
	"time"

	"killa/server/internal/protocol"
)

// Match lifecycle states.
const (
	MatchWaiting   = "waiting"
	MatchCountdown = "countdown"
	MatchLive      = "live"
	MatchEnded     = "ended"

	NoWinner   = -1
	DrawWinner = 2
)

const (
	ScoreLimit        = 40
	CountdownDuration = 3 * time.Second
	MatchDuration     = 10 * time.Minute
	ResultDuration    = 8 * time.Second
)

// updateMatch advances the match state machine. It is called once per tick.
func (r *Room) updateMatch(now time.Time) {
	switch r.matchState {
	case MatchWaiting:
		if r.canStartMatch() {
			r.matchState = MatchCountdown
			r.stateEndsAt = now.Add(CountdownDuration)
		}

	case MatchCountdown:
		if !r.canStartMatch() {
			r.abortMatch()
			return
		}
		if !now.Before(r.stateEndsAt) {
			r.beginMatch(now)
		}

	case MatchLive:
		if len(r.players) < 2 {
			r.abortMatch()
			return
		}
		if !now.Before(r.matchEndsAt) {
			r.finishMatch(now)
		}

	case MatchEnded:
		if !now.Before(r.stateEndsAt) {
			r.matchState = MatchWaiting
			r.score = [2]int{}
			r.winner = NoWinner
			if r.canStartMatch() {
				r.matchState = MatchCountdown
				r.stateEndsAt = now.Add(CountdownDuration)
			}
		}
	}
}

func (r *Room) canStartMatch() bool {
	if len(r.players) < 2 {
		return false
	}
	teams := [2]int{}
	for _, p := range r.players {
		if p.Team == 0 || p.Team == 1 {
			teams[p.Team]++
		}
	}
	return teams[0] > 0 && teams[1] > 0
}

func (r *Room) beginMatch(now time.Time) {
	r.invincible = false
	log.Printf("killa: %s match starting (%d players, %v)", r.ID, len(r.players), r.matchDuration)
	r.matchState = MatchLive
	r.matchEndsAt = now.Add(r.matchDuration)
	r.score = [2]int{}
	r.winner = NoWinner
	r.resetPlayersForMatch(now)
}

func (r *Room) finishMatch(now time.Time) {
	r.invincible = true
	log.Printf("killa: %s match ended %d:%d", r.ID, r.score[0], r.score[1])
	r.matchState = MatchEnded
	r.stateEndsAt = now.Add(ResultDuration)
	switch {
	case r.score[0] > r.score[1]:
		r.winner = 0
	case r.score[1] > r.score[0]:
		r.winner = 1
	default:
		r.winner = DrawWinner
	}
}

func (r *Room) abortMatch() {
	r.invincible = false
	r.matchState = MatchWaiting
	r.stateEndsAt = time.Time{}
	r.score = [2]int{}
	r.winner = NoWinner
}

func (r *Room) resetPlayersForMatch(now time.Time) {
	used := [2]int{}
	for _, p := range r.players {
		spawn := r.spawnFor(p.Team, used[p.Team])
		used[p.Team]++
		p.State = State{X: spawn.X, Z: spawn.Z, OnGround: true, Yaw: spawn.Yaw}
		p.HP = MaxHealth
		p.Alive = true
		p.Kills = 0
		p.Deaths = 0
		p.RespawnAt = time.Time{}
		p.ProtectedUntil = now.Add(SpawnProtectionTime)
		p.History = p.History[:0]
		p.LastShot = time.Time{}
		p.Damagers = nil
		p.LastDamaged = time.Time{}
		p.Assists = 0
		r.equip(p)
		if p.Brain != nil {
			p.Brain.targetID = ""
			p.Brain.path = nil
			p.Brain.nextThink = now
			p.Brain.nextPath = now
		}
	}
}

func (r *Room) matchSnapshot(now time.Time) protocol.Match {
	state := r.matchState
	if state == "" {
		state = MatchWaiting
	}
	m := protocol.Match{
		State:  state,
		Score:  r.score,
		Winner: r.winner,
	}

	switch state {
	case MatchCountdown, MatchEnded:
		if !r.stateEndsAt.IsZero() {
			m.Countdown = math.Max(0, r.stateEndsAt.Sub(now).Seconds())
		}
	case MatchLive:
		if !r.matchEndsAt.IsZero() {
			m.SecondsLeft = math.Max(0, r.matchEndsAt.Sub(now).Seconds())
		}
	}
	return m
}
