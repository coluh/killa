package game

import "time"

// PlayerResult is one human participant's contribution to a finished match.
type PlayerResult struct {
	ProfileID string
	Name      string
	Team      int
	Kills     int
	Deaths    int
	Assists   int
	Damage    int
	Headshots int
}

// MatchResult is emitted once when a match ends and players are still present.
type MatchResult struct {
	ID        string
	RoomID    string
	StartedAt time.Time
	EndedAt   time.Time
	Score     [2]int
	Winner    int
	Players   []PlayerResult
}

// SetMatchRecorder installs a callback for finished matches.
func (r *Room) SetMatchRecorder(fn func(MatchResult)) {
	r.matchRecorder = fn
}

func (r *Room) recordMatch(now time.Time) {
	if r.matchRecorder == nil {
		return
	}
	started := r.startedAt
	if started.IsZero() {
		started = now.Add(-r.matchDuration)
	}
	result := MatchResult{
		ID:        newID(),
		RoomID:    r.ID,
		StartedAt: started,
		EndedAt:   now,
		Score:     r.score,
		Winner:    r.winner,
	}
	for _, p := range r.players {
		if p.ProfileID == "" {
			continue
		}
		result.Players = append(result.Players, PlayerResult{
			ProfileID: p.ProfileID,
			Name:      p.Name,
			Team:      p.Team,
			Kills:     p.Kills,
			Deaths:    p.Deaths,
			Assists:   p.Assists,
			Damage:    p.Damage,
			Headshots: p.Headshots,
		})
	}
	if len(result.Players) == 0 {
		return
	}
	r.matchRecorder(result)
}
