package game

import (
	"time"

	"killa/server/internal/protocol"
)

// Weapon is the server-side tuning of one gun.
type Weapon struct {
	ID           string
	Name         string
	Damage       int
	HeadDamage   int
	FireInterval time.Duration
	MagSize      int
	ReloadTime   time.Duration
	Spread       float64
	Range        float64
	Recoil       float64
}

const DefaultWeapon = "ar"

var weaponOrder = []string{"ar", "smg"}

var weaponTable = map[string]Weapon{
	"ar": {
		ID:           "ar",
		Name:         "M416",
		Damage:       41,
		HeadDamage:   94,
		FireInterval: 100 * time.Millisecond,
		MagSize:      30,
		ReloadTime:   1800 * time.Millisecond,
		Spread:       0.004,
		Range:        80,
		Recoil:       0.0035,
	},
	"smg": {
		ID:           "smg",
		Name:         "UZI",
		Damage:       26,
		HeadDamage:   55,
		FireInterval: 70 * time.Millisecond,
		MagSize:      25,
		ReloadTime:   1500 * time.Millisecond,
		Spread:       0.012,
		Range:        45,
		Recoil:       0.002,
	},
}

func weaponByID(id string) Weapon {
	if w, ok := weaponTable[id]; ok {
		return w
	}
	return weaponTable[DefaultWeapon]
}

func WeaponInfos() []protocol.Weapon {
	out := make([]protocol.Weapon, 0, len(weaponOrder))
	for _, id := range weaponOrder {
		w := weaponTable[id]
		out = append(out, protocol.Weapon{
			ID:             w.ID,
			Name:           w.Name,
			Damage:         w.Damage,
			HeadDamage:     w.HeadDamage,
			FireIntervalMs: int(w.FireInterval / time.Millisecond),
			MagSize:        w.MagSize,
			ReloadMs:       int(w.ReloadTime / time.Millisecond),
			Spread:         w.Spread,
			Range:          w.Range,
			Recoil:         w.Recoil,
		})
	}
	return out
}
