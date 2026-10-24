package game

import (
	"math"

	"killa/server/internal/protocol"
)

// State is the authoritative kinematic state of a player.
type State struct {
	X        float64
	Y        float64
	Z        float64
	VY       float64
	OnGround bool
	Yaw      float64
	Pitch    float64
}

// Step advances one player by one input. It must stay byte-for-byte equivalent
// to step() in client/src/world.ts.
func Step(s *State, in protocol.Input, w *World) {
	dt := in.Dt
	if math.IsNaN(dt) || dt < 0 {
		dt = 0
	}
	if dt > MaxDt {
		dt = MaxDt
	}

	s.Yaw = in.Yaw
	s.Pitch = clamp(in.Pitch, -PitchLimit, PitchLimit)

	forward := in.Forward
	right := in.Right
	magnitude := math.Hypot(forward, right)
	if magnitude > 1 {
		forward /= magnitude
		right /= magnitude
	}

	speed := WalkSpeed
	if in.Sprint {
		speed = SprintSpeed
	}

	sin, cos := math.Sincos(s.Yaw)
	dx := (-forward*sin + right*cos) * speed * dt
	dz := (-forward*cos - right*sin) * speed * dt

	if nx := s.X + dx; !w.blockedAt(nx, s.Z, PlayerRadius, s.Y) {
		s.X = nx
	}
	if nz := s.Z + dz; !w.blockedAt(s.X, nz, PlayerRadius, s.Y) {
		s.Z = nz
	}
	s.X, s.Z = w.withinBounds(s.X, s.Z, PlayerRadius)

	if in.Jump && s.OnGround {
		s.VY = JumpSpeed
		s.OnGround = false
	}
	prevY := s.Y
	s.VY -= Gravity * dt
	s.Y += s.VY * dt

	// Use the pre-gravity height so a fast fall cannot tunnel through a crate top.
	support := w.supportHeight(s.X, s.Z, PlayerRadius, prevY)
	if s.Y <= support {
		s.Y = support
		s.VY = 0
		s.OnGround = true
	} else {
		s.OnGround = false
	}
}
