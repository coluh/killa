package game

import (
	"testing"
)

func TestReferenceMapSpawnsAndPaths(t *testing.T) {
	world := NewWarehouseReference()

	if len(world.Spawns) != 8 {
		t.Fatalf("spawns = %d, want 8", len(world.Spawns))
	}

	nav := buildNav(world)
	for _, spawn := range world.Spawns {
		if world.blocked(spawn.X, spawn.Z, PlayerRadius) {
			t.Fatalf("spawn inside crate: %+v", spawn)
		}
		path := nav.findPath(Vec2{X: spawn.X, Z: spawn.Z}, Vec2{X: 0, Z: 0})
		if len(path) == 0 {
			t.Fatalf("no path from spawn %+v to center", spawn)
		}
	}
}
