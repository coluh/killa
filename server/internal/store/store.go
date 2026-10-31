// Package store persists profiles, sessions and match history in SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

const schema = `
CREATE TABLE IF NOT EXISTS profiles (
	id              TEXT PRIMARY KEY,
	username        TEXT,
	username_lower  TEXT UNIQUE,
	password_hash   TEXT,
	display_name    TEXT NOT NULL,
	is_guest        INTEGER NOT NULL DEFAULT 1,
	created_at      INTEGER NOT NULL,
	settings_json   TEXT NOT NULL DEFAULT '{}',
	loadout         TEXT NOT NULL DEFAULT 'ar',
	total_kills     INTEGER NOT NULL DEFAULT 0,
	total_deaths    INTEGER NOT NULL DEFAULT 0,
	total_assists   INTEGER NOT NULL DEFAULT 0,
	total_damage    INTEGER NOT NULL DEFAULT 0,
	total_headshots INTEGER NOT NULL DEFAULT 0,
	matches_played  INTEGER NOT NULL DEFAULT 0,
	wins            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_profile ON sessions(profile_id);

CREATE TABLE IF NOT EXISTS matches (
	id               TEXT PRIMARY KEY,
	room_id          TEXT NOT NULL,
	started_at       INTEGER NOT NULL,
	ended_at         INTEGER NOT NULL,
	score0           INTEGER NOT NULL,
	score1           INTEGER NOT NULL,
	winner           INTEGER NOT NULL,
	duration_seconds INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS match_players (
	match_id   TEXT NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
	profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
	team       INTEGER NOT NULL,
	kills      INTEGER NOT NULL DEFAULT 0,
	deaths     INTEGER NOT NULL DEFAULT 0,
	assists    INTEGER NOT NULL DEFAULT 0,
	damage     INTEGER NOT NULL DEFAULT 0,
	headshots  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (match_id, profile_id)
);
CREATE INDEX IF NOT EXISTS match_players_profile ON match_players(profile_id);
`

// Store is a small SQLite-backed repository.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the database at path.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite has a single writer; serializing keeps small deployments simple.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	_ = os.Chmod(path, 0o600)

	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Stats is the lifetime aggregate shown on the profile page.
type Stats struct {
	Kills     int `json:"kills"`
	Deaths    int `json:"deaths"`
	Assists   int `json:"assists"`
	Damage    int `json:"damage"`
	Headshots int `json:"headshots"`
	Matches   int `json:"matches"`
	Wins      int `json:"wins"`
}

// Profile is a player identity. Password hashes never leave the store.
type Profile struct {
	ID          string
	Username    string
	DisplayName string
	IsGuest     bool
	CreatedAt   time.Time
	Settings    string
	Loadout     string
	Stats       Stats
}

const profileColumns = `
	id, COALESCE(username, ''), display_name, is_guest, created_at,
	settings_json, loadout, total_kills, total_deaths, total_assists,
	total_damage, total_headshots, matches_played, wins`

func scanProfile(row interface{ Scan(...any) error }) (*Profile, error) {
	var p Profile
	var created int64
	var guest int
	err := row.Scan(
		&p.ID, &p.Username, &p.DisplayName, &guest, &created,
		&p.Settings, &p.Loadout, &p.Stats.Kills, &p.Stats.Deaths,
		&p.Stats.Assists, &p.Stats.Damage, &p.Stats.Headshots,
		&p.Stats.Matches, &p.Stats.Wins,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.IsGuest = guest != 0
	p.CreatedAt = time.Unix(created, 0)
	return &p, nil
}

// CreateGuest inserts a passwordless profile.
func (s *Store) CreateGuest(id, displayName string, now time.Time) (*Profile, error) {
	_, err := s.db.Exec(`
		INSERT INTO profiles (id, username, username_lower, password_hash, display_name, is_guest, created_at)
		VALUES (?, NULL, NULL, NULL, ?, 1, ?)`,
		id, displayName, now.Unix())
	if err != nil {
		return nil, err
	}
	return s.ProfileByID(id)
}

// CreateUser inserts a named profile with a password hash.
func (s *Store) CreateUser(id, username, usernameLower, displayName, passwordHash string, now time.Time) (*Profile, error) {
	_, err := s.db.Exec(`
		INSERT INTO profiles (id, username, username_lower, password_hash, display_name, is_guest, created_at)
		VALUES (?, ?, ?, ?, ?, 0, ?)`,
		id, username, usernameLower, passwordHash, displayName, now.Unix())
	if err != nil {
		return nil, err
	}
	return s.ProfileByID(id)
}

// FindByUsername returns the profile and its password hash.
func (s *Store) FindByUsername(usernameLower string) (*Profile, string, error) {
	var hash string
	row := s.db.QueryRow(`SELECT `+profileColumns+`, COALESCE(password_hash, '') FROM profiles WHERE username_lower = ?`, usernameLower)
	p, err := scanProfileWithHash(row, &hash)
	if err != nil {
		return nil, "", err
	}
	return p, hash, nil
}

func scanProfileWithHash(row interface{ Scan(...any) error }, hash *string) (*Profile, error) {
	var p Profile
	var created int64
	var guest int
	err := row.Scan(
		&p.ID, &p.Username, &p.DisplayName, &guest, &created,
		&p.Settings, &p.Loadout, &p.Stats.Kills, &p.Stats.Deaths,
		&p.Stats.Assists, &p.Stats.Damage, &p.Stats.Headshots,
		&p.Stats.Matches, &p.Stats.Wins, hash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.IsGuest = guest != 0
	p.CreatedAt = time.Unix(created, 0)
	return &p, nil
}

func (s *Store) ProfileByID(id string) (*Profile, error) {
	return scanProfile(s.db.QueryRow(`SELECT `+profileColumns+` FROM profiles WHERE id = ?`, id))
}

// SetCredentials binds a username and password to an existing profile (guest upgrade).
func (s *Store) SetCredentials(id, username, usernameLower, displayName, passwordHash string) error {
	res, err := s.db.Exec(`
		UPDATE profiles
		SET username = ?, username_lower = ?, display_name = ?, password_hash = ?, is_guest = 0
		WHERE id = ? AND is_guest = 1`,
		username, usernameLower, displayName, passwordHash, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetPassword(id, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE profiles SET password_hash = ? WHERE id = ? AND is_guest = 0`, passwordHash, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateSettings(id, settings, loadout string) error {
	res, err := s.db.Exec(`UPDATE profiles SET settings_json = ?, loadout = ? WHERE id = ?`, settings, loadout, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProfile(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM profiles WHERE id = ?`, id); err != nil {
		return err
	}
	// Remove matches that no longer have any participant.
	if _, err := tx.Exec(`DELETE FROM matches WHERE NOT EXISTS (
		SELECT 1 FROM match_players WHERE match_players.match_id = matches.id
	)`); err != nil {
		return err
	}
	return tx.Commit()
}

// Sessions.

func (s *Store) CreateSession(profileID, tokenHash string, now, expires time.Time) error {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now.Unix())
	_, err := s.db.Exec(`
		INSERT INTO sessions (token_hash, profile_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)`,
		tokenHash, profileID, now.Unix(), expires.Unix())
	return err
}

func (s *Store) SessionProfile(tokenHash string, now time.Time) (*Profile, error) {
	return scanProfile(s.db.QueryRow(`
		SELECT `+profileColumns+` FROM profiles
		WHERE id = (SELECT profile_id FROM sessions WHERE token_hash = ? AND expires_at > ?)`,
		tokenHash, now.Unix()))
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) DeleteSessions(profileID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE profile_id = ?`, profileID)
	return err
}

func (s *Store) DeleteOtherSessions(profileID, keepTokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE profile_id = ? AND token_hash <> ?`, profileID, keepTokenHash)
	return err
}

// Match history.

type MatchPlayer struct {
	ProfileID string
	Team      int
	Kills     int
	Deaths    int
	Assists   int
	Damage    int
	Headshots int
}

type MatchInput struct {
	ID        string
	RoomID    string
	StartedAt time.Time
	EndedAt   time.Time
	Score     [2]int
	Winner    int
	Players   []MatchPlayer
}

// RecordMatch stores one finished match and updates lifetime aggregates atomically.
func (s *Store) RecordMatch(m MatchInput) error {
	if m.ID == "" || len(m.Players) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	duration := int(m.EndedAt.Sub(m.StartedAt).Seconds())
	if duration < 0 {
		duration = 0
	}

	res, err := tx.Exec(`
		INSERT OR IGNORE INTO matches (id, room_id, started_at, ended_at, score0, score1, winner, duration_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.RoomID, m.StartedAt.Unix(), m.EndedAt.Unix(), m.Score[0], m.Score[1], m.Winner, duration)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}

	for _, p := range m.Players {
		if p.ProfileID == "" {
			continue
		}
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, p.ProfileID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO match_players
			(match_id, profile_id, team, kills, deaths, assists, damage, headshots)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, p.ProfileID, p.Team, p.Kills, p.Deaths, p.Assists, p.Damage, p.Headshots); err != nil {
			return err
		}

		wins := 0
		if m.Winner == p.Team {
			wins = 1
		}
		if _, err := tx.Exec(`
			UPDATE profiles SET
				total_kills = total_kills + ?,
				total_deaths = total_deaths + ?,
				total_assists = total_assists + ?,
				total_damage = total_damage + ?,
				total_headshots = total_headshots + ?,
				matches_played = matches_played + 1,
				wins = wins + ?
			WHERE id = ?`,
			p.Kills, p.Deaths, p.Assists, p.Damage, p.Headshots, wins, p.ProfileID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

type MatchRecord struct {
	ID        string
	RoomID    string
	EndedAt   time.Time
	Score     [2]int
	Winner    int
	Team      int
	Kills     int
	Deaths    int
	Assists   int
	Damage    int
	Headshots int
}

func (s *Store) RecentMatches(profileID string, limit int) ([]MatchRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT m.id, m.room_id, m.ended_at, m.score0, m.score1, m.winner,
		       mp.team, mp.kills, mp.deaths, mp.assists, mp.damage, mp.headshots
		FROM match_players mp JOIN matches m ON m.id = mp.match_id
		WHERE mp.profile_id = ?
		ORDER BY m.ended_at DESC
		LIMIT ?`,
		profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MatchRecord, 0, limit)
	for rows.Next() {
		var r MatchRecord
		var ended int64
		if err := rows.Scan(&r.ID, &r.RoomID, &ended, &r.Score[0], &r.Score[1], &r.Winner,
			&r.Team, &r.Kills, &r.Deaths, &r.Assists, &r.Damage, &r.Headshots); err != nil {
			return nil, err
		}
		r.EndedAt = time.Unix(ended, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}
