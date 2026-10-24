package game

import (
	"math"
	"testing"

	"killa/server/internal/protocol"
)

func openWorld() *World {
	return &World{Bounds: protocol.Bounds{MinX: -50, MaxX: 50, MinZ: -50, MaxZ: 50}}
}

func TestWalkForward(t *testing.T) {
	w := openWorld()
	s := State{X: 0, Z: 10, OnGround: true}
	in := protocol.Input{Dt: 0.05, Forward: 1}

	for i := 0; i < 20; i++ {
		Step(&s, in, w)
	}

	if math.Abs(s.Z-5.2) > 0.001 {
		t.Fatalf("z = %v, want 5.2", s.Z)
	}
	if math.Abs(s.X) > 0.001 {
		t.Fatalf("x drifted to %v", s.X)
	}
}

func TestSprint(t *testing.T) {
	w := openWorld()
	s := State{X: 0, Z: 10, OnGround: true}
	in := protocol.Input{Dt: 0.05, Forward: 1, Sprint: true}

	for i := 0; i < 20; i++ {
		Step(&s, in, w)
	}

	if math.Abs(s.Z-3.4) > 0.001 {
		t.Fatalf("z = %v, want 3.4", s.Z)
	}
}

func TestDiagonalIsNormalized(t *testing.T) {
	w := openWorld()
	s := State{X: 0, Z: 10, OnGround: true}
	in := protocol.Input{Dt: 0.05, Forward: 1, Right: 1}

	for i := 0; i < 20; i++ {
		Step(&s, in, w)
	}

	diagonal := 4.8 / math.Sqrt2
	if math.Abs(s.Z-(10-diagonal)) > 0.001 || math.Abs(s.X-diagonal) > 0.001 {
		t.Fatalf("position = (%v, %v), want (%v, %v)", s.X, s.Z, diagonal, 10-diagonal)
	}
}

func TestCrateBlocks(t *testing.T) {
	w := NewWarehouse()
	s := State{X: -2.5, Z: 0, OnGround: true}
	in := protocol.Input{Dt: 0.05, Forward: 1, Yaw: -math.Pi / 2}

	for i := 0; i < 40; i++ {
		Step(&s, in, w)
	}

	if s.X <= -2.1 {
		t.Fatalf("player did not move: x = %v", s.X)
	}
	if s.X > -1.89 {
		t.Fatalf("player entered the crate: x = %v", s.X)
	}
}

func TestBoundsClamp(t *testing.T) {
	w := openWorld()
	s := State{X: 0, Z: 49.5, OnGround: true}
	in := protocol.Input{Dt: 0.05, Forward: 1, Yaw: math.Pi}

	for i := 0; i < 20; i++ {
		Step(&s, in, w)
	}

	if math.Abs(s.Z-49.6) > 0.001 {
		t.Fatalf("z = %v, want clamp at 49.6", s.Z)
	}
}

func TestJumpAndLand(t *testing.T) {
	w := openWorld()
	s := State{X: 0, Z: 0, OnGround: true}

	Step(&s, protocol.Input{Dt: 0.05, Jump: true}, w)
	if s.Y <= 0 || s.OnGround {
		t.Fatalf("expected airborne, y = %v onGround = %v", s.Y, s.OnGround)
	}

	for i := 0; i < 100; i++ {
		Step(&s, protocol.Input{Dt: 0.05}, w)
	}
	if s.Y != 0 || !s.OnGround {
		t.Fatalf("expected landed, y = %v onGround = %v", s.Y, s.OnGround)
	}
}

func TestInvalidDtIgnored(t *testing.T) {
	w := openWorld()
	s := State{X: 0, Z: 10, OnGround: true}
	in := protocol.Input{Dt: math.NaN(), Forward: 1}

	Step(&s, in, w)

	if s.X != 0 || s.Z != 10 {
		t.Fatalf("NaN dt moved player to (%v, %v)", s.X, s.Z)
	}
}

func TestCrateSupportsStanding(t *testing.T) {
	w := &World{
		Bounds: protocol.Bounds{MinX: -50, MaxX: 50, MinZ: -50, MaxZ: 50},
		Crates: []protocol.Crate{{X: 0, Z: -2, W: 4, D: 4, H: 1.0}},
	}

	if !w.blockedAt(0, -2, PlayerRadius, 0) {
		t.Fatal("crate should block a grounded player")
	}
	if w.blockedAt(0, -2, PlayerRadius, 1.05) {
		t.Fatal("crate should not block a player standing on it")
	}
	if got := w.supportHeight(0, -2, PlayerRadius, 1.1); got != 1.0 {
		t.Fatalf("support = %v, want 1.0", got)
	}

	s := State{X: 0, Z: -2, Y: 1.5, OnGround: false}
	for i := 0; i < 40; i++ {
		Step(&s, protocol.Input{Dt: 0.05}, w)
	}
	if math.Abs(s.Y-1.0) > 0.001 || !s.OnGround {
		t.Fatalf("landed at y=%v ground=%v, want y=1.0", s.Y, s.OnGround)
	}
}

func TestFallingOffCrate(t *testing.T) {
	w := &World{
		Bounds: protocol.Bounds{MinX: -50, MaxX: 50, MinZ: -50, MaxZ: 50},
		Crates: []protocol.Crate{{X: 0, Z: -2, W: 4, D: 4, H: 1.0}},
	}
	s := State{X: 0, Z: 3.2, Y: 1.0, OnGround: true}

	for i := 0; i < 60; i++ {
		Step(&s, protocol.Input{Dt: 0.05}, w)
	}
	if s.Y != 0 || !s.OnGround {
		t.Fatalf("expected floor, got y=%v ground=%v", s.Y, s.OnGround)
	}
}

func TestJumpOntoCrate(t *testing.T) {
	w := &World{
		Bounds: protocol.Bounds{MinX: -50, MaxX: 50, MinZ: -50, MaxZ: 50},
		Crates: []protocol.Crate{{X: 0, Z: -5.5, W: 6, D: 10, H: 1.0}},
	}
	s := State{X: 0, Z: 0.2, OnGround: true}

	Step(&s, protocol.Input{Dt: 0.05, Forward: 1, Jump: true}, w)
	for i := 0; i < 20; i++ {
		Step(&s, protocol.Input{Dt: 0.05, Forward: 1}, w)
	}

	if math.Abs(s.Y-1.0) > 0.01 {
		t.Fatalf("y = %v, want standing on crate at 1.0", s.Y)
	}
	if s.Z > -0.5 {
		t.Fatalf("z = %v, want player inside crate footprint", s.Z)
	}
}

func TestFastFallDoesNotTunnel(t *testing.T) {
	w := &World{
		Bounds: protocol.Bounds{MinX: -50, MaxX: 50, MinZ: -50, MaxZ: 50},
		Crates: []protocol.Crate{{X: 0, Z: 0, W: 6, D: 6, H: 1.0}},
	}
	s := State{X: 0, Z: 0, Y: 4, VY: -20}

	for i := 0; i < 10; i++ {
		Step(&s, protocol.Input{Dt: 0.05}, w)
	}
	if math.Abs(s.Y-1.0) > 0.001 || !s.OnGround {
		t.Fatalf("y = %v ground = %v, want crate top 1.0", s.Y, s.OnGround)
	}
}
