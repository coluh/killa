package game

import (
	"math"
	"testing"
	"time"

	"killa/server/internal/protocol"
)

type fakePeer struct {
	sent []any
}

func (f *fakePeer) Send(v any) error {
	f.sent = append(f.sent, v)
	return nil
}

func (f *fakePeer) Close() error { return nil }

func newCombatRoom() (*Room, *Player, *Player) {
	world := &World{
		Bounds: protocol.Bounds{MinX: -50, MaxX: 50, MinZ: -50, MaxZ: 50},
		Spawns: []protocol.Spawn{
			{Team: 0, X: 0, Z: 10, Yaw: 0},
			{Team: 1, X: 0, Z: -10, Yaw: math.Pi},
		},
	}
	room := NewRoom("combat", world)

	p1 := &Player{ID: "p1", Name: "one", Team: 0, HP: MaxHealth, Alive: true, State: State{Z: 0}, Peer: &fakePeer{}, Loadout: "ar", Weapon: "ar", Ammo: 30}
	p2 := &Player{ID: "p2", Name: "two", Team: 1, HP: MaxHealth, Alive: true, State: State{Z: -5}, Peer: &fakePeer{}, Loadout: "ar", Weapon: "ar", Ammo: 30}

	room.players[p1.ID] = p1
	room.players[p2.ID] = p2
	room.byPeer[p1.Peer] = p1
	room.byPeer[p2.Peer] = p2

	return room, p1, p2
}

func TestRayCylinder(t *testing.T) {
	tests := []struct {
		name string
		oy   float64
		cx   float64
		want bool
		head bool
	}{
		{name: "head", oy: 1.6, cx: 0, want: true, head: true},
		{name: "body", oy: 1.0, cx: 0, want: true, head: false},
		{name: "miss", oy: 1.6, cx: 2, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dist, hit, head := rayCylinder(0, tc.oy, 0, 0, 0, -1, 0, tc.cx, -5, PlayerRadius, PlayerHeight, HeadHeight)
			if hit != tc.want {
				t.Fatalf("hit = %v, want %v", hit, tc.want)
			}
			if !tc.want {
				return
			}
			if head != tc.head {
				t.Fatalf("head = %v, want %v", head, tc.head)
			}
			if math.Abs(dist-4.6) > 0.001 {
				t.Fatalf("dist = %v, want 4.6", dist)
			}
		})
	}
}

func TestRayAABB(t *testing.T) {
	dist, ok := rayAABB(0, 1.6, 0, 0, 0, -1, -2, 0, -3, 2, 2, -2)
	if !ok || math.Abs(dist-2) > 0.001 {
		t.Fatalf("dist = %v ok = %v, want 2", dist, ok)
	}
}

func TestShotDamagesEnemy(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	room.applyShot(p1, 0, 0, time.Now())

	if p2.HP != MaxHealth-HeadDamage {
		t.Fatalf("hp = %d, want %d", p2.HP, MaxHealth-HeadDamage)
	}
	if p1.Kills != 0 || !p2.Alive {
		t.Fatalf("unexpected kill state: kills=%d alive=%v", p1.Kills, p2.Alive)
	}
}

func TestFriendlyFireIgnored(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	p2.Team = p1.Team
	room.applyShot(p1, 0, 0, time.Now())

	if p2.HP != MaxHealth {
		t.Fatalf("friendly fire dealt damage: hp = %d", p2.HP)
	}
}

func TestWallBlocksShot(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	room.world.Crates = []protocol.Crate{{X: 0, Z: -2.5, W: 4, D: 1, H: 2}}
	room.applyShot(p1, 0, 0, time.Now())

	if p2.HP != MaxHealth {
		t.Fatalf("shot passed through wall: hp = %d", p2.HP)
	}
}

func TestFireInterval(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	now := time.Now()
	room.applyShot(p1, 0, 0, now)
	room.applyShot(p1, 0, 0, now.Add(50*time.Millisecond))

	if want := MaxHealth - HeadDamage; p2.HP != want {
		t.Fatalf("hp = %d, want %d (second shot should be rate-limited)", p2.HP, want)
	}
}

func TestKillAndRespawn(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	now := time.Now()

	for i := 0; i < 3; i++ {
		room.applyShot(p1, 0, 0, now.Add(time.Duration(i)*120*time.Millisecond))
	}

	if p2.Alive {
		t.Fatalf("victim still alive with hp = %d", p2.HP)
	}
	if p1.Kills != 1 {
		t.Fatalf("kills = %d, want 1", p1.Kills)
	}
	if p2.Deaths != 1 {
		t.Fatalf("deaths = %d, want 1", p2.Deaths)
	}

	room.respawnDead(now.Add(RespawnDelay + 500*time.Millisecond))

	if !p2.Alive || p2.HP != MaxHealth {
		t.Fatalf("respawn failed: alive=%v hp=%d", p2.Alive, p2.HP)
	}
	if p2.State.Z != -10 {
		t.Fatalf("respawn position z = %v, want -10", p2.State.Z)
	}
}

func TestSpawnProtectionBlocksDamage(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	now := time.Now()
	p2.ProtectedUntil = now.Add(SpawnProtectionTime)
	room.applyShot(p1, 0, 0, now)

	if p2.HP != MaxHealth {
		t.Fatalf("protected player took damage: hp = %d", p2.HP)
	}
}

func TestPositionAtInterpolates(t *testing.T) {
	p := &Player{}
	base := time.Unix(1000, 0)
	p.History = []PosSample{
		{T: base, Z: 0},
		{T: base.Add(50 * time.Millisecond), Z: 5},
	}
	_, _, z := p.positionAt(base.Add(25 * time.Millisecond))
	if math.Abs(z-2.5) > 0.001 {
		t.Fatalf("z = %v, want 2.5", z)
	}
}

func TestVestAbsorbsBodyDamage(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	p2.Vest = 100
	room.applyShot(p1, 0, -0.1, time.Now())

	if p2.HP != MaxHealth-16 {
		t.Fatalf("hp = %d, want %d", p2.HP, MaxHealth-16)
	}
	if p2.Vest != 92 {
		t.Fatalf("vest = %d, want 92", p2.Vest)
	}
}

func TestHelmetAbsorbsHeadDamage(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	p2.Helmet = 100
	room.applyShot(p1, 0, 0, time.Now())

	if p2.HP != MaxHealth-32 {
		t.Fatalf("hp = %d, want %d", p2.HP, MaxHealth-32)
	}
	if p2.Helmet != 84 {
		t.Fatalf("helmet = %d, want 84", p2.Helmet)
	}
}

func TestReloadFlow(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	p1.Ammo = 1
	now := time.Now()

	room.applyShot(p1, 0, 0, now)
	if p1.Ammo != 0 || !p1.Reloading {
		t.Fatalf("after last shot: ammo=%d reloading=%v", p1.Ammo, p1.Reloading)
	}
	hp := p2.HP

	room.applyShot(p1, 0, 0, now.Add(200*time.Millisecond))
	if p2.HP != hp {
		t.Fatal("shot fired while reloading")
	}

	room.completeReloads(now.Add(3 * time.Second))
	if p1.Ammo != weaponByID("ar").MagSize || p1.Reloading {
		t.Fatalf("reload failed: ammo=%d reloading=%v", p1.Ammo, p1.Reloading)
	}
}

func TestEquipLoadout(t *testing.T) {
	room, p1, _ := newCombatRoom()
	p1.Loadout = "smg"
	room.equip(p1)

	if p1.Weapon != "smg" {
		t.Fatalf("weapon = %q, want smg", p1.Weapon)
	}
	if p1.Ammo != weaponByID("smg").MagSize {
		t.Fatalf("ammo = %d, want %d", p1.Ammo, weaponByID("smg").MagSize)
	}
	if p1.Vest != 100 || p1.Helmet != 100 {
		t.Fatalf("armor = %d/%d, want 100/100", p1.Vest, p1.Helmet)
	}
}

func TestSMGLowerRange(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	p1.Loadout = "smg"
	room.equip(p1)
	p2.State.Z = -50

	room.applyShot(p1, 0, 0, time.Now())

	if p2.HP != MaxHealth {
		t.Fatalf("smg hit beyond range: hp = %d", p2.HP)
	}
}

func TestLoadoutAppliedOnRespawn(t *testing.T) {
	room, p1, _ := newCombatRoom()
	p1.Loadout = "smg"
	p1.Alive = false
	p1.RespawnAt = time.Now().Add(-time.Second)

	room.respawnDead(time.Now())

	if p1.Weapon != "smg" {
		t.Fatalf("weapon = %q, want smg", p1.Weapon)
	}
	if p1.Ammo != weaponByID("smg").MagSize {
		t.Fatalf("ammo = %d, want %d", p1.Ammo, weaponByID("smg").MagSize)
	}
	if p1.Vest != 100 || p1.Helmet != 100 {
		t.Fatalf("armor = %d/%d, want 100/100", p1.Vest, p1.Helmet)
	}
}

func TestJoinWithWeapon(t *testing.T) {
	room, _, _ := newCombatRoom()
	resp := make(chan error, 1)
	room.join(joinReq{peer: &fakePeer{}, name: "smg-player", weapon: "smg", resp: resp})
	if err := <-resp; err != nil {
		t.Fatalf("join: %v", err)
	}

	var joined *Player
	for _, p := range room.players {
		if p.Name == "smg-player" {
			joined = p
		}
	}
	if joined == nil {
		t.Fatal("joined player not found")
	}
	if joined.Weapon != "smg" || joined.Ammo != weaponByID("smg").MagSize {
		t.Fatalf("weapon = %q ammo = %d", joined.Weapon, joined.Ammo)
	}

	resp2 := make(chan error, 1)
	room.join(joinReq{peer: &fakePeer{}, name: "bad-weapon", weapon: "awm", resp: resp2})
	if err := <-resp2; err != nil {
		t.Fatalf("join bad weapon: %v", err)
	}
	for _, p := range room.players {
		if p.Name == "bad-weapon" && p.Weapon != DefaultWeapon {
			t.Fatalf("invalid weapon fell back to %q", p.Weapon)
		}
	}
}

func TestAssistOnKill(t *testing.T) {
	room, p1, p2 := newCombatRoom()
	now := time.Now()

	p3 := &Player{
		ID: "p3", Name: "three", Team: 0, HP: MaxHealth, Alive: true,
		Peer: &fakePeer{}, Loadout: "ar", Weapon: "ar", Ammo: 30,
	}
	room.players[p3.ID] = p3
	room.byPeer[p3.Peer] = p3

	room.applyShot(p3, 0, -0.1, now)
	if p2.HP >= MaxHealth {
		t.Fatalf("assist damage did not land: hp = %d", p2.HP)
	}

	room.applyShot(p1, 0, 0, now.Add(200*time.Millisecond))
	room.applyShot(p1, 0, 0, now.Add(400*time.Millisecond))

	if p2.Alive {
		t.Fatalf("victim still alive with hp = %d", p2.HP)
	}
	if p3.Assists != 1 {
		t.Fatalf("assists = %d, want 1", p3.Assists)
	}
	if p1.Kills != 1 {
		t.Fatalf("kills = %d, want 1", p1.Kills)
	}
}
