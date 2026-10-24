package game

import (
	"math"
	"testing"
	"time"
)

func TestMatchLifecycle(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	room.matchState = MatchWaiting
	room.winner = NoWinner

	now := time.Unix(1000, 0)
	room.updateMatch(now)
	if room.matchState != MatchCountdown {
		t.Fatalf("state = %q, want countdown", room.matchState)
	}
	if got := room.stateEndsAt.Sub(now); got != CountdownDuration {
		t.Fatalf("countdown = %v, want %v", got, CountdownDuration)
	}

	p1.Kills = 3
	p2.Deaths = 2
	room.updateMatch(now.Add(CountdownDuration))

	if room.matchState != MatchLive {
		t.Fatalf("state = %q, want live", room.matchState)
	}
	if p1.Kills != 0 || p2.Deaths != 0 {
		t.Fatalf("stats not reset: kills=%d deaths=%d", p1.Kills, p2.Deaths)
	}
	if !p1.Alive || p1.HP != MaxHealth {
		t.Fatalf("player not reset: alive=%v hp=%d", p1.Alive, p1.HP)
	}
	if p1.State.Z != 10 || p2.State.Z != -10 {
		t.Fatalf("unexpected spawn positions: %v / %v", p1.State.Z, p2.State.Z)
	}
	if got := room.matchEndsAt.Sub(now.Add(CountdownDuration)); got != MatchDuration {
		t.Fatalf("match duration = %v, want %v", got, MatchDuration)
	}
}

func TestKillScoresDuringLive(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	now := time.Unix(1000, 0)
	room.matchState = MatchLive
	room.matchEndsAt = now.Add(MatchDuration)

	room.kill(p1, p2, now)

	if room.score[0] != 1 || room.score[1] != 0 {
		t.Fatalf("score = %v, want [1 0]", room.score)
	}
	if p1.Kills != 1 || p2.Deaths != 1 {
		t.Fatalf("stats = %d/%d, want 1/1", p1.Kills, p2.Deaths)
	}
	if room.matchState != MatchLive {
		t.Fatalf("state = %q, want live", room.matchState)
	}
}

func TestScoreLimitEndsMatch(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	now := time.Unix(1000, 0)
	room.matchState = MatchLive
	room.matchEndsAt = now.Add(MatchDuration)
	room.score[0] = ScoreLimit - 1

	room.kill(p1, p2, now)

	if room.score[0] != ScoreLimit {
		t.Fatalf("score = %d, want %d", room.score[0], ScoreLimit)
	}
	if room.matchState != MatchEnded {
		t.Fatalf("state = %q, want ended", room.matchState)
	}
	if room.winner != 0 {
		t.Fatalf("winner = %d, want 0", room.winner)
	}
}

func TestTimeLimitPicksHigherScore(t *testing.T) {
	room, _, _ := newCombatRoom()
	now := time.Unix(1000, 0)
	room.matchState = MatchLive
	room.matchEndsAt = now.Add(-time.Second)
	room.score = [2]int{5, 3}

	room.updateMatch(now)

	if room.matchState != MatchEnded || room.winner != 0 {
		t.Fatalf("state = %q winner = %d, want ended/0", room.matchState, room.winner)
	}
}

func TestTimeLimitTieIsDraw(t *testing.T) {
	room, _, _ := newCombatRoom()
	now := time.Unix(1000, 0)
	room.matchState = MatchLive
	room.matchEndsAt = now.Add(-time.Second)
	room.score = [2]int{4, 4}

	room.updateMatch(now)

	if room.matchState != MatchEnded || room.winner != DrawWinner {
		t.Fatalf("state = %q winner = %d, want ended/draw", room.matchState, room.winner)
	}
}

func TestKillOutsideLiveDoesNotScore(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	room.matchState = MatchWaiting
	now := time.Unix(1000, 0)

	room.kill(p1, p2, now)

	if room.score[0] != 0 {
		t.Fatalf("score = %d, want 0", room.score[0])
	}
	if p1.Kills != 1 {
		t.Fatalf("personal kills = %d, want 1", p1.Kills)
	}
}

func TestMatchRestartsAfterResult(t *testing.T) {
	room, _, _ := newCombatRoom()
	now := time.Unix(1000, 0)
	room.matchState = MatchEnded
	room.winner = 0
	room.score = [2]int{40, 12}
	room.stateEndsAt = now

	room.updateMatch(now.Add(time.Millisecond))

	if room.score != [2]int{0, 0} {
		t.Fatalf("score = %v, want reset", room.score)
	}
	if room.winner != NoWinner {
		t.Fatalf("winner = %d, want none", room.winner)
	}
	if room.matchState != MatchCountdown {
		t.Fatalf("state = %q, want countdown", room.matchState)
	}
}

func TestMatchAbortsWhenPlayersLeave(t *testing.T) {
	room, p1, _ := newCombatRoom()
	now := time.Unix(1000, 0)
	room.matchState = MatchLive
	room.score = [2]int{3, 1}
	room.matchEndsAt = now.Add(MatchDuration)

	delete(room.players, "p2")
	room.updateMatch(now)

	if room.matchState != MatchWaiting {
		t.Fatalf("state = %q, want waiting", room.matchState)
	}
	if room.score != [2]int{0, 0} {
		t.Fatalf("score = %v, want reset", room.score)
	}
	_ = p1
}

func TestMatchSnapshotTimes(t *testing.T) {
	room, _, _ := newCombatRoom()
	now := time.Unix(1000, 0)

	room.matchState = MatchCountdown
	room.stateEndsAt = now.Add(2 * time.Second)
	snap := room.matchSnapshot(now)
	if math.Abs(snap.Countdown-2) > 0.001 {
		t.Fatalf("countdown = %v, want 2", snap.Countdown)
	}

	room.matchState = MatchLive
	room.matchEndsAt = now.Add(90 * time.Second)
	snap = room.matchSnapshot(now)
	if math.Abs(snap.SecondsLeft-90) > 0.001 {
		t.Fatalf("secondsLeft = %v, want 90", snap.SecondsLeft)
	}

	room.matchState = MatchEnded
	room.stateEndsAt = now.Add(5 * time.Second)
	snap = room.matchSnapshot(now)
	if snap.State != MatchEnded || math.Abs(snap.Countdown-5) > 0.001 {
		t.Fatalf("ended snapshot = %+v", snap)
	}
}
