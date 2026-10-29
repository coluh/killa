# 通信协议（M1）

WebSocket 文本帧，JSON。生产环境走 `wss://<host>/ws`。

## 通用

- 单条消息上限 4KB。
- 客户端第一帧必须是 `join`，否则断开。
- 所有浮点字段必须有限；服务端会截断非法值。

## 客户端 → 服务端

### join

```json
{ "type": "join", "name": "Player-1234", "weapon": "ar" }
```

`name` 可选，最长 16 字符，空则服务端生成。`weapon` 可选（`ar` / `smg`），
用于在加入时直接装备本地保存的配装；非法值回落到默认武器。

客户端断线后会自动重连（指数退避，最多 5 次），重连等同一次新的 `join`。

### input

每个渲染帧发送一次（约 60Hz）。

```json
{
  "type": "input",
  "seq": 42,
  "dt": 0.016,
  "forward": 1,
  "right": 0,
  "sprint": false,
  "jump": false,
  "yaw": 1.57,
  "pitch": 0.1
}
```

| 字段 | 说明 |
| --- | --- |
| `seq` | 单调递增，用于快照确认（ack）与预测重放 |
| `dt` | 距上一帧的秒数，服务端截断到 [0, 0.1] |
| `forward` / `right` | [-1, 1]，服务端归一化 |
| `sprint` | 是否疾跑 |
| `jump` | 本帧是否起跳 |
| `yaw` / `pitch` | 朝向弧度；pitch 限制 ±1.5 |
| `rtt` | 客户端测得的往返延迟（毫秒），服务端截断到 [0, 250]，用于命中回溯 |
| `ads` | 是否开镜；开镜时移速 ×0.55、扩散 ×0.35 |

### loadout

选择主武器，**下一次复活生效**。服务端只接受武器表中的 ID。

```json
{ "type": "loadout", "weapon": "smg" }
```

### reload

手动换弹；弹匣打空时服务端也会自动开始换弹。

```json
{ "type": "reload" }
```

### shoot

左键开火。服务端按射速（100ms）限流，从眼睛位置沿 `yaw/pitch` 做 hitscan；
箱子会挡住子弹。

```json
{ "type": "shoot", "yaw": 0.0, "pitch": 0.0 }
```

- 伤害、射速、弹匣、换弹、扩散、射程由武器表决定。
- 护甲/头盔各吸收 35% 对应部位伤害并消耗耐久；复活时重置为 100。
- 服务端会按射手的 RTT/2 回溯目标位置，缓解高延迟下“打不中”的问题。
- 击杀后 3 秒在己方出生点复活，并获得 1.5 秒出生保护。

### ping

```json
{ "type": "ping", "t": 1729800000000 }
```

`t` 为客户端时间戳（毫秒），服务端原样回传。

## 服务端 → 客户端

### welcome

加入成功后第一帧，随后立即跟一帧 `snapshot`。

```json
{
  "type": "welcome",
  "playerId": "a1b2c3d4",
  "roomId": "arena-1",
  "team": 0,
  "tickRate": 20,
  "map": {
    "bounds": { "minX": -20, "maxX": 20, "minZ": -20, "maxZ": 20 },
    "crates": [{ "x": 0, "z": 0, "w": 3, "d": 3, "h": 1.2 }],
    "spawns": [{ "team": 0, "x": -2, "z": 16, "yaw": 0 }]
  },
  "weapons": [
    { "id": "ar", "name": "M416", "damage": 24, "headDamage": 48, "fireIntervalMs": 100, "magSize": 30, "reloadMs": 1800, "spread": 0.004, "range": 80, "recoil": 0.0035 },
    { "id": "smg", "name": "UZI", "damage": 18, "headDamage": 36, "fireIntervalMs": 70, "magSize": 25, "reloadMs": 1500, "spread": 0.012, "range": 45, "recoil": 0.002 }
  ]
}
```

### snapshot

20Hz 广播；每个玩家收到的 `ack` 是自己的。

```json
{
  "type": "snapshot",
  "tick": 1024,
  "ack": 41,
  "match": {
    "state": "live",
    "score": [12, 9],
    "secondsLeft": 523.4,
    "countdown": 0,
    "winner": -1
  },
  "players": [
    {
      "id": "a1b2c3d4",
      "name": "Player-1",
      "team": 0,
      "x": 0, "y": 0, "z": 16,
      "yaw": 0, "pitch": 0,
      "hp": 100,
      "alive": true,
      "kills": 0,
      "deaths": 0,
      "respawnIn": 0,
      "weapon": "ar",
      "ammo": 30,
      "reloading": false,
      "reloadIn": 0,
      "vest": 100,
      "helmet": 100
    }
  ]
}
```

- `respawnIn`：死亡后剩余复活秒数，存活时为 0。
- `weapon` / `ammo`：当前武器与剩余弹药。
- `reloading` / `reloadIn`：是否换弹中、剩余换弹秒数。
- `vest` / `helmet`：护甲/头盔耐久（0–100）。
- `ads`：该玩家当前是否开镜。
- `assists`：本局助攻数（对击杀者之外、5 秒内造成过伤害的敌人计数）。

### match

| 字段 | 说明 |
| --- | --- |
| `state` | `waiting` / `countdown` / `live` / `ended` |
| `score` | `[蓝队击杀数, 橙队击杀数]` |
| `secondsLeft` | 进行中剩余秒数（仅 `live`） |
| `countdown` | 开局或结算剩余秒数（`countdown` / `ended`） |
| `winner` | `-1` 未定 / `0` 蓝队 / `1` 橙队 / `2` 平局 |

比赛规则：至少 2 人且双方都有玩家时进入 3 秒倒计时；先到 40 杀或 10 分钟
结束时击杀更多的一方获胜；结算 8 秒后自动重置并开始下一局。`-match-seconds`
和 `-score-limit` 可在开发/自定义房间时覆盖默认值。

### shot

广播给房间内所有玩家，用于渲染曳光与命中反馈。

```json
{
  "type": "shot",
  "shooter": "a1b2c3d4",
  "x": 1.0, "y": 1.6, "z": 16.0,
  "endX": 1.0, "endY": 1.6, "endZ": -16.0,
  "hit": "b2c3d4e5",
  "headshot": true
}
```

`hit` 为空表示未命中（打墙或空枪）。

### kill

```json
{
  "type": "kill",
  "killer": "a1b2c3d4",
  "killerName": "Player-1",
  "victim": "b2c3d4e5",
  "victimName": "Player-2"
}
```

### pong

```json
{ "type": "pong", "t": 1729800000000 }
```

### error

```json
{ "type": "error", "message": "room is full" }
```

发送后服务端断开连接。

## 错误与关闭

| 场景 | 行为 |
| --- | --- |
| 首帧不是 join / 解析失败 | 发送 `error` 后关闭 |
| 房间已满（8 人） | 发送 `error` 后关闭 |
| 开火过于频繁 / 已死亡 | 服务端忽略该次射击 |
| 读超时（120s 无消息） | 关闭 |
| 连接异常 | 清理玩家，广播后续快照 |
