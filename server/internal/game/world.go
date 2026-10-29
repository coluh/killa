package game

import "killa/server/internal/protocol"

// World holds the static geometry of a map.
type World struct {
	Bounds protocol.Bounds
	Crates []protocol.Crate
	Spawns []protocol.Spawn
}

// NewWarehouse builds the M1 blockout: a symmetric hall with a mid corridor,
// side cover and two spawn areas.
func NewWarehouse() *World {
	return &World{
		Bounds: protocol.Bounds{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
		Crates: []protocol.Crate{
			// 中央长墙，形成中路走廊
			{X: -6, Z: 0, W: 2, D: 10, H: 2.4},
			{X: 6, Z: 0, W: 2, D: 10, H: 2.4},
			// 中央掩体
			{X: 0, Z: 0, W: 3, D: 3, H: 1.2},
			// 南北横挡
			{X: 0, Z: -7, W: 6, D: 1.5, H: 1.2},
			{X: 0, Z: 7, W: 6, D: 1.5, H: 1.2},
			// 四角箱堆
			{X: -11, Z: -11, W: 3, D: 3, H: 1.6},
			{X: 11, Z: -11, W: 3, D: 3, H: 1.6},
			{X: -11, Z: 11, W: 3, D: 3, H: 1.6},
			{X: 11, Z: 11, W: 3, D: 3, H: 1.6},
			// 出生区前的屏障，留出左右两个出口，挡住出生点直射
			{X: 0, Z: 12.5, W: 4, D: 1, H: 2.4},
			{X: -8.5, Z: 12.5, W: 5, D: 1, H: 2.4},
			{X: 8.5, Z: 12.5, W: 5, D: 1, H: 2.4},
			{X: 0, Z: -12.5, W: 4, D: 1, H: 2.4},
			{X: -8.5, Z: -12.5, W: 5, D: 1, H: 2.4},
			{X: 8.5, Z: -12.5, W: 5, D: 1, H: 2.4},
			// 出生区后方的矮掩体
			{X: 0, Z: 18.5, W: 3, D: 1.5, H: 1.0},
			{X: 0, Z: -18.5, W: 3, D: 1.5, H: 1.0},
			// 外场掩体
			{X: -15, Z: 8, W: 2, D: 2, H: 1.2},
			{X: 15, Z: 8, W: 2, D: 2, H: 1.2},
			{X: -15, Z: -8, W: 2, D: 2, H: 1.2},
			{X: 15, Z: -8, W: 2, D: 2, H: 1.2},
			{X: -16, Z: 16, W: 3, D: 3, H: 1.6},
			{X: 16, Z: 16, W: 3, D: 3, H: 1.6},
			{X: -16, Z: -16, W: 3, D: 3, H: 1.6},
			{X: 16, Z: -16, W: 3, D: 3, H: 1.6},
			// 可跳上去的低平台
			{X: 0, Z: 4.5, W: 2.5, D: 2.5, H: 1.0},
			{X: 0, Z: -4.5, W: 2.5, D: 2.5, H: 1.0},
			{X: -9, Z: 6, W: 2, D: 2, H: 1.0},
			{X: 9, Z: -6, W: 2, D: 2, H: 1.0},
			{X: -9, Z: -6, W: 2, D: 2, H: 1.0},
			{X: 9, Z: 6, W: 2, D: 2, H: 1.0},
		},
		Spawns: []protocol.Spawn{
			// 0 队（蓝）南侧，面向 -Z
			{Team: 0, X: -2, Z: 16, Yaw: 0},
			{Team: 0, X: 2, Z: 16, Yaw: 0},
			{Team: 0, X: -5, Z: 16, Yaw: 0},
			{Team: 0, X: 5, Z: 16, Yaw: 0},
			// 1 队（橙）北侧，面向 +Z
			{Team: 1, X: -2, Z: -16, Yaw: 3.141592653589793},
			{Team: 1, X: 2, Z: -16, Yaw: 3.141592653589793},
			{Team: 1, X: -5, Z: -16, Yaw: 3.141592653589793},
			{Team: 1, X: 5, Z: -16, Yaw: 3.141592653589793},
		},
	}
}

// ProtoMap returns a copy of the map data safe to serialize.
func (w *World) ProtoMap() protocol.Map {
	return protocol.Map{
		Bounds: w.Bounds,
		Crates: append([]protocol.Crate(nil), w.Crates...),
		Spawns: append([]protocol.Spawn(nil), w.Spawns...),
	}
}

// blocked reports whether a ground-level circle at (x, z) intersects a crate.
func (w *World) blocked(x, z, r float64) bool {
	return w.blockedAt(x, z, r, 0)
}

// blockedAt is like blocked, but crates whose top is at or below the player's
// feet no longer block horizontal movement (they can be walked/jumped onto).
func (w *World) blockedAt(x, z, r, feetY float64) bool {
	r2 := r * r
	for _, c := range w.Crates {
		if feetY+0.05 >= c.H {
			continue
		}
		minX := c.X - c.W/2
		maxX := c.X + c.W/2
		minZ := c.Z - c.D/2
		maxZ := c.Z + c.D/2

		cx := clamp(x, minX, maxX)
		cz := clamp(z, minZ, maxZ)
		dx := x - cx
		dz := z - cz
		if dx*dx+dz*dz < r2 {
			return true
		}
	}
	return false
}

// supportHeight returns the highest crate top at or below the player's feet,
// i.e. the surface the player would land on. The floor is height 0.
func (w *World) supportHeight(x, z, r, feetY float64) float64 {
	best := 0.0
	r2 := r * r
	for _, c := range w.Crates {
		if c.H > feetY+0.05 {
			continue
		}
		minX := c.X - c.W/2
		maxX := c.X + c.W/2
		minZ := c.Z - c.D/2
		maxZ := c.Z + c.D/2

		cx := clamp(x, minX, maxX)
		cz := clamp(z, minZ, maxZ)
		dx := x - cx
		dz := z - cz
		if dx*dx+dz*dz < r2 && c.H > best {
			best = c.H
		}
	}
	return best
}

// withinBounds clamps a position to the playable area, keeping the player radius.
func (w *World) withinBounds(x, z, r float64) (float64, float64) {
	return clamp(x, w.Bounds.MinX+r, w.Bounds.MaxX-r),
		clamp(z, w.Bounds.MinZ+r, w.Bounds.MaxZ-r)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// NewWarehouseReference ports the first-floor blockout sketched in
// myold/tdm world_ofDebug, scaled 1.5x and with ramps/spheres omitted.
func NewWarehouseReference() *World {
	return &World{
		Bounds: protocol.Bounds{MinX: -15, MaxX: 15, MinZ: -30, MaxZ: 30},
		Crates: []protocol.Crate{
			{X: 10.88, Z: -27.38, W: 8.25, D: 5.25, H: 3.00},
			{X: 13.12, Z: -18.38, W: 3.75, D: 3.75, H: 2.50},
			{X: -3.38, Z: -25.50, W: 6.75, D: 9.00, H: 0.50},
			{X: 3.00, Z: -18.45, W: 6.00, D: 5.10, H: 2.50},
			{X: 7.12, Z: -15.67, W: 2.25, D: 0.45, H: 2.50},
			{X: 9.75, Z: -18.23, W: 3.00, D: 0.45, H: 2.50},
			{X: -6.60, Z: -18.23, W: 0.30, D: 5.55, H: 2.40},
			{X: -3.38, Z: -18.23, W: 6.75, D: 5.55, H: 0.10},
			{X: -8.25, Z: -15.67, W: 3.00, D: 0.45, H: 2.50},
			{X: -11.85, Z: -20.62, W: 2.70, D: 2.25, H: 1.50},
			{X: -12.38, Z: -3.38, W: 5.25, D: 3.75, H: 2.50},
			{X: -13.35, Z: -18.75, W: 0.30, D: 7.50, H: 2.50},
			{X: -13.35, Z: -14.62, W: 0.30, D: 0.75, H: 2.00},
			{X: -13.35, Z: -9.75, W: 0.30, D: 9.00, H: 2.50},
			{X: -6.38, Z: -10.12, W: 5.25, D: 2.25, H: 2.50},
			{X: 6.75, Z: -8.25, W: 4.50, D: 3.00, H: 2.50},
			{X: 2.03, Z: -8.77, W: 0.45, D: 5.55, H: 2.50},
			{X: 3.15, Z: -11.77, W: 2.70, D: 0.45, H: 2.50},
			{X: 12.75, Z: -2.25, W: 4.50, D: 6.00, H: 2.50},
			{X: 12.00, Z: 2.25, W: 6.00, D: 3.00, H: 2.50},
			{X: 13.20, Z: 5.85, W: 0.30, D: 4.20, H: 2.50},
			{X: 13.20, Z: 8.48, W: 0.30, D: 1.05, H: 2.00},
			{X: 13.20, Z: 15.38, W: 0.30, D: 12.75, H: 2.50},
			{X: 11.77, Z: 19.12, W: 2.55, D: 2.25, H: 2.00},
			{X: 6.38, Z: 9.38, W: 5.25, D: 2.25, H: 2.50},
			{X: -3.75, Z: 9.00, W: 3.00, D: 3.00, H: 2.50},
			{X: -5.25, Z: 5.25, W: 3.00, D: 4.50, H: 2.50},
			{X: -7.50, Z: 4.12, W: 1.50, D: 2.25, H: 2.50},
			{X: -10.50, Z: 14.17, W: 6.00, D: 0.45, H: 2.50},
			{X: -14.25, Z: 12.00, W: 1.50, D: 6.00, H: 3.00},
			{X: -13.88, Z: 3.75, W: 2.25, D: 10.50, H: 2.50},
			{X: -12.38, Z: 27.38, W: 5.25, D: 5.25, H: 3.00},
			{X: 8.25, Z: 15.67, W: 3.00, D: 0.45, H: 2.50},
			{X: 6.60, Z: 18.23, W: 0.30, D: 5.55, H: 2.40},
			{X: 3.38, Z: 18.23, W: 6.75, D: 5.55, H: 0.10},
			{X: -3.75, Z: 19.50, W: 7.50, D: 3.00, H: 2.50},
			{X: 0.15, Z: 18.23, W: 0.30, D: 5.55, H: 2.40},
			{X: 3.38, Z: 25.50, W: 6.75, D: 9.00, H: 0.50},
		},
		Spawns: []protocol.Spawn{
			// 南侧
			{Team: 0, X: -8, Z: 28.5, Yaw: 0},
			{Team: 0, X: -4, Z: 28.5, Yaw: 0},
			{Team: 0, X: 9, Z: 28.5, Yaw: 0},
			{Team: 0, X: 13, Z: 28.5, Yaw: 0},
			// 北侧
			{Team: 1, X: -13, Z: -28.5, Yaw: 3.141592653589793},
			{Team: 1, X: -9, Z: -28.5, Yaw: 3.141592653589793},
			{Team: 1, X: 2, Z: -28.5, Yaw: 3.141592653589793},
			{Team: 1, X: 5, Z: -28.5, Yaw: 3.141592653589793},
		},
	}
}
