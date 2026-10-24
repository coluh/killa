package game

import (
	"container/heap"
	"math"
)

// Vec2 is a point on the horizontal plane.
type Vec2 struct {
	X float64
	Z float64
}

// navGrid is a coarse walkability grid built from the map crates.
type navGrid struct {
	minX, minZ float64
	cell       float64
	cols, rows int
	blocked    []bool
}

func buildNav(world *World) *navGrid {
	const cell = 1.0
	minX := math.Floor(world.Bounds.MinX)
	minZ := math.Floor(world.Bounds.MinZ)
	cols := int(math.Ceil((world.Bounds.MaxX - minX) / cell))
	rows := int(math.Ceil((world.Bounds.MaxZ - minZ) / cell))

	g := &navGrid{
		minX:    minX,
		minZ:    minZ,
		cell:    cell,
		cols:    cols,
		rows:    rows,
		blocked: make([]bool, cols*rows),
	}
	for iz := 0; iz < rows; iz++ {
		for ix := 0; ix < cols; ix++ {
			cx := minX + (float64(ix)+0.5)*cell
			cz := minZ + (float64(iz)+0.5)*cell
			g.blocked[iz*cols+ix] = world.blocked(cx, cz, PlayerRadius+0.15)
		}
	}
	return g
}

func (g *navGrid) index(ix, iz int) int { return iz*g.cols + ix }

func (g *navGrid) inBounds(ix, iz int) bool {
	return ix >= 0 && ix < g.cols && iz >= 0 && iz < g.rows
}

func (g *navGrid) free(ix, iz int) bool {
	return g.inBounds(ix, iz) && !g.blocked[g.index(ix, iz)]
}

func (g *navGrid) cellAt(x, z float64) (int, int) {
	ix := int(math.Floor((x - g.minX) / g.cell))
	iz := int(math.Floor((z - g.minZ) / g.cell))
	return clampInt(ix, 0, g.cols-1), clampInt(iz, 0, g.rows-1)
}

func (g *navGrid) center(ix, iz int) Vec2 {
	return Vec2{
		X: g.minX + (float64(ix)+0.5)*g.cell,
		Z: g.minZ + (float64(iz)+0.5)*g.cell,
	}
}

func (g *navGrid) nearestFree(ix, iz int) (int, int) {
	if g.free(ix, iz) {
		return ix, iz
	}
	for radius := 1; radius <= 6; radius++ {
		for dz := -radius; dz <= radius; dz++ {
			for dx := -radius; dx <= radius; dx++ {
				if absInt(dx) != radius && absInt(dz) != radius {
					continue
				}
				if g.free(ix+dx, iz+dz) {
					return ix + dx, iz + dz
				}
			}
		}
	}
	return ix, iz
}

func (g *navGrid) heuristic(ax, az, bx, bz int) float64 {
	dx := absInt(ax - bx)
	dz := absInt(az - bz)
	return float64(maxInt(dx, dz)) + (math.Sqrt2-1)*float64(minInt(dx, dz))
}

// findPath returns waypoints (cell centers) from start towards goal.
func (g *navGrid) findPath(start, goal Vec2) []Vec2 {
	sx, sz := g.cellAt(start.X, start.Z)
	gx, gz := g.cellAt(goal.X, goal.Z)
	sx, sz = g.nearestFree(sx, sz)
	gx, gz = g.nearestFree(gx, gz)

	startIdx := g.index(sx, sz)
	goalIdx := g.index(gx, gz)
	if startIdx == goalIdx {
		return []Vec2{goal}
	}

	n := len(g.blocked)
	gScore := make([]float64, n)
	for i := range gScore {
		gScore[i] = math.Inf(1)
	}
	came := make([]int, n)
	for i := range came {
		came[i] = -1
	}
	closed := make([]bool, n)

	gScore[startIdx] = 0
	open := &astarHeap{{idx: startIdx, f: g.heuristic(sx, sz, gx, gz)}}
	heap.Init(open)

	dirs := [8][2]int{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
	}

	for open.Len() > 0 {
		item := heap.Pop(open).(astarItem)
		if closed[item.idx] {
			continue
		}
		closed[item.idx] = true
		if item.idx == goalIdx {
			break
		}

		cx, cz := item.idx%g.cols, item.idx/g.cols
		for _, d := range dirs {
			nx, nz := cx+d[0], cz+d[1]
			if !g.free(nx, nz) {
				continue
			}
			if d[0] != 0 && d[1] != 0 {
				if !g.free(cx+d[0], cz) || !g.free(cx, cz+d[1]) {
					continue
				}
			}

			step := 1.0
			if d[0] != 0 && d[1] != 0 {
				step = math.Sqrt2
			}
			ni := g.index(nx, nz)
			ng := gScore[item.idx] + step
			if ng >= gScore[ni] {
				continue
			}
			gScore[ni] = ng
			came[ni] = item.idx
			heap.Push(open, astarItem{idx: ni, f: ng + g.heuristic(nx, nz, gx, gz)})
		}
	}

	if came[goalIdx] == -1 {
		return nil
	}

	path := make([]Vec2, 0, 16)
	for cur := goalIdx; cur != startIdx; cur = came[cur] {
		if cur == -1 {
			return nil
		}
		cx, cz := cur%g.cols, cur/g.cols
		path = append(path, g.center(cx, cz))
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

type astarItem struct {
	idx int
	f   float64
}

type astarHeap []astarItem

func (h astarHeap) Len() int           { return len(h) }
func (h astarHeap) Less(i, j int) bool { return h[i].f < h[j].f }
func (h astarHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *astarHeap) Push(x any)        { *h = append(*h, x.(astarItem)) }
func (h *astarHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
