// Package protocol defines the JSON messages exchanged between client and server.
//
// Keep this in sync with client/src/net.ts and docs/protocol.md.
package protocol

// Envelope is used to peek at the message type before decoding the full message.
type Envelope struct {
	Type string `json:"type"`
}

type Join struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Weapon string `json:"weapon"`
}

type Input struct {
	Type    string  `json:"type"`
	Seq     uint32  `json:"seq"`
	Dt      float64 `json:"dt"`
	Forward float64 `json:"forward"`
	Right   float64 `json:"right"`
	Sprint  bool    `json:"sprint"`
	Jump    bool    `json:"jump"`
	Yaw     float64 `json:"yaw"`
	Pitch   float64 `json:"pitch"`
	RTT     float64 `json:"rtt"`
	Ads     bool    `json:"ads"`
}

type Shoot struct {
	Type  string  `json:"type"`
	Yaw   float64 `json:"yaw"`
	Pitch float64 `json:"pitch"`
}

type Loadout struct {
	Type   string `json:"type"`
	Weapon string `json:"weapon"`
}

type Reload struct {
	Type string `json:"type"`
}

type Shot struct {
	Type     string  `json:"type"`
	Shooter  string  `json:"shooter"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	EndX     float64 `json:"endX"`
	EndY     float64 `json:"endY"`
	EndZ     float64 `json:"endZ"`
	Hit      string  `json:"hit,omitempty"`
	Headshot bool    `json:"headshot,omitempty"`
}

type Kill struct {
	Type       string `json:"type"`
	Killer     string `json:"killer"`
	KillerName string `json:"killerName"`
	Victim     string `json:"victim"`
	VictimName string `json:"victimName"`
}

type Ping struct {
	Type string  `json:"type"`
	T    float64 `json:"t"`
}

type Pong struct {
	Type string  `json:"type"`
	T    float64 `json:"t"`
}

type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type Welcome struct {
	Type     string   `json:"type"`
	PlayerID string   `json:"playerId"`
	RoomID   string   `json:"roomId"`
	Team     int      `json:"team"`
	TickRate int      `json:"tickRate"`
	Map      Map      `json:"map"`
	Weapons  []Weapon `json:"weapons"`
}

type Weapon struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Damage         int     `json:"damage"`
	HeadDamage     int     `json:"headDamage"`
	FireIntervalMs int     `json:"fireIntervalMs"`
	MagSize        int     `json:"magSize"`
	ReloadMs       int     `json:"reloadMs"`
	Spread         float64 `json:"spread"`
	Range          float64 `json:"range"`
	Recoil         float64 `json:"recoil"`
}

type Map struct {
	Bounds Bounds  `json:"bounds"`
	Crates []Crate `json:"crates"`
	Spawns []Spawn `json:"spawns"`
}

type Bounds struct {
	MinX float64 `json:"minX"`
	MaxX float64 `json:"maxX"`
	MinZ float64 `json:"minZ"`
	MaxZ float64 `json:"maxZ"`
}

type Crate struct {
	X float64 `json:"x"`
	Z float64 `json:"z"`
	W float64 `json:"w"`
	D float64 `json:"d"`
	H float64 `json:"h"`
}

type Spawn struct {
	Team int     `json:"team"`
	X    float64 `json:"x"`
	Z    float64 `json:"z"`
	Yaw  float64 `json:"yaw"`
}

type Snapshot struct {
	Type    string   `json:"type"`
	Tick    uint64   `json:"tick"`
	Ack     uint32   `json:"ack"`
	Match   Match    `json:"match"`
	Players []Player `json:"players"`
}

type Match struct {
	State       string  `json:"state"`
	Score       [2]int  `json:"score"`
	SecondsLeft float64 `json:"secondsLeft"`
	Countdown   float64 `json:"countdown"`
	Winner      int     `json:"winner"`
}

type Player struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Team      int     `json:"team"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Yaw       float64 `json:"yaw"`
	Pitch     float64 `json:"pitch"`
	HP        int     `json:"hp"`
	Alive     bool    `json:"alive"`
	Kills     int     `json:"kills"`
	Deaths    int     `json:"deaths"`
	RespawnIn float64 `json:"respawnIn"`
	Weapon    string  `json:"weapon"`
	Ammo      int     `json:"ammo"`
	Reloading bool    `json:"reloading"`
	ReloadIn  float64 `json:"reloadIn"`
	Vest      int     `json:"vest"`
	Helmet    int     `json:"helmet"`
	Ads       bool    `json:"ads"`
	Assists   int     `json:"assists"`
}
