package game

import "time"

// Simulation constants. The client mirrors these in client/src/world.ts and
// client/src/net.ts — keep both sides in sync or prediction will drift.
const (
	TickRate = 20

	MaxPlayers = 8
	TeamSize   = 4

	PlayerRadius = 0.4
	EyeHeight    = 1.6
	PlayerHeight = 1.7
	HeadHeight   = 1.35

	WalkSpeed   = 4.8
	SprintSpeed = 6.6

	Gravity   = 20.0
	JumpSpeed = 7.2

	MaxDt      = 0.1
	PitchLimit = 1.5

	MaxNameRunes = 16

	MaxHealth  = 100
	BodyDamage = 24
	HeadDamage = 48

	FireInterval        = 100 * time.Millisecond
	RespawnDelay        = 3 * time.Second
	SpawnProtectionTime = 1500 * time.Millisecond
)
