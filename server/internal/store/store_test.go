package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestGuestSessionAndProfile(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	p, err := s.CreateGuest("guest-1", "Guest-ABCD", now)
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsGuest || p.Username != "" || p.DisplayName != "Guest-ABCD" {
		t.Fatalf("unexpected profile: %+v", p)
	}

	if err := s.CreateSession(p.ID, "hash-1", now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionProfile("hash-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID {
		t.Fatalf("session profile = %s, want %s", got.ID, p.ID)
	}
	if _, err := s.SessionProfile("hash-1", now.Add(2*time.Hour)); err != ErrNotFound {
		t.Fatalf("expired session error = %v", err)
	}
}

func TestUserCredentialsAndUpgrade(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	u, err := s.CreateUser("u1", "Alice", "alice", "Alice", "pw-hash", now)
	if err != nil {
		t.Fatal(err)
	}
	if u.IsGuest || u.Username != "Alice" {
		t.Fatalf("unexpected user: %+v", u)
	}
	got, hash, err := s.FindByUsername("alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID || hash != "pw-hash" {
		t.Fatalf("find mismatch: %+v %q", got, hash)
	}

	if _, err := s.CreateUser("u2", "Alice", "alice", "Alice", "x", now); err == nil {
		t.Fatal("duplicate username should fail")
	}

	g, err := s.CreateGuest("g1", "Guest-0001", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCredentials(g.ID, "Bob", "bob", "Bob", "bob-hash"); err != nil {
		t.Fatal(err)
	}
	bob, _, err := s.FindByUsername("bob")
	if err != nil || bob.ID != g.ID || bob.IsGuest {
		t.Fatalf("upgrade failed: %+v %v", bob, err)
	}
}

func TestRecordMatchStats(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	a, _ := s.CreateUser("a", "A", "a", "A", "h", now)
	b, _ := s.CreateUser("b", "B", "b", "B", "h", now)

	start := now.Add(-10 * time.Minute)
	err := s.RecordMatch(MatchInput{
		ID: "m1", RoomID: "room-1", StartedAt: start, EndedAt: now,
		Score: [2]int{40, 20}, Winner: 0,
		Players: []MatchPlayer{
			{ProfileID: a.ID, Team: 0, Kills: 12, Deaths: 4, Assists: 2, Damage: 800, Headshots: 5},
			{ProfileID: b.ID, Team: 1, Kills: 4, Deaths: 12, Assists: 1, Damage: 300, Headshots: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ap, err := s.ProfileByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ap.Stats.Kills != 12 || ap.Stats.Matches != 1 || ap.Stats.Wins != 1 {
		t.Fatalf("stats = %+v", ap.Stats)
	}
	bp, _ := s.ProfileByID(b.ID)
	if bp.Stats.Wins != 0 || bp.Stats.Matches != 1 {
		t.Fatalf("loser stats = %+v", bp.Stats)
	}

	// Duplicate writes are ignored.
	if err := s.RecordMatch(MatchInput{ID: "m1", Players: []MatchPlayer{{ProfileID: a.ID}}}); err != nil {
		t.Fatal(err)
	}
	ap, _ = s.ProfileByID(a.ID)
	if ap.Stats.Matches != 1 {
		t.Fatalf("duplicate match changed stats: %+v", ap.Stats)
	}

	recent, err := s.RecentMatches(a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Kills != 12 || recent[0].Score != [2]int{40, 20} {
		t.Fatalf("recent = %+v", recent)
	}

	if err := s.DeleteProfile(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProfileByID(a.ID); err != ErrNotFound {
		t.Fatalf("deleted profile error = %v", err)
	}
	recent, _ = s.RecentMatches(a.ID, 10)
	if len(recent) != 0 {
		t.Fatalf("deleted profile still has matches: %+v", recent)
	}
}

func TestRecordMatchSkipsDeletedProfile(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	a, _ := s.CreateUser("a", "A", "a", "A", "h", now)
	b, _ := s.CreateUser("b", "B", "b", "B", "h", now)
	if err := s.DeleteProfile(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordMatch(MatchInput{
		ID: "m-del", RoomID: "r", StartedAt: now, EndedAt: now, Winner: 0,
		Players: []MatchPlayer{{ProfileID: a.ID, Team: 0}, {ProfileID: b.ID, Team: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	ap, _ := s.ProfileByID(a.ID)
	if ap.Stats.Matches != 1 {
		t.Fatalf("surviving profile matches = %d", ap.Stats.Matches)
	}
}
