package game

import (
	"testing"

	"killa/server/internal/protocol"
)

func TestNavPathAroundWall(t *testing.T) {
	world := &World{
		Bounds: protocol.Bounds{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
		Crates: []protocol.Crate{{X: 0, Z: 0, W: 10, D: 2, H: 2}},
	}
	nav := buildNav(world)

	path := nav.findPath(Vec2{X: 0, Z: 6}, Vec2{X: 0, Z: -6})
	if len(path) == 0 {
		t.Fatal("no path found")
	}
	for _, wp := range path {
		if world.blocked(wp.X, wp.Z, PlayerRadius) {
			t.Fatalf("waypoint inside crate: %+v", wp)
		}
	}
}

func TestLineOfSight(t *testing.T) {
	world := &World{
		Bounds: protocol.Bounds{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
		Crates: []protocol.Crate{{X: 0, Z: 0, W: 10, D: 2, H: 2}},
	}

	if world.lineOfSight(0, 1.6, 6, 0, 1.6, -6) {
		t.Fatal("line of sight should be blocked")
	}
	if !world.lineOfSight(6.5, 1.6, 6, 6.5, 1.6, -6) {
		t.Fatal("line of sight should be clear beside the crate")
	}
}
