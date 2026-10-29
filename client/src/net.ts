export interface Bounds {
  minX: number
  maxX: number
  minZ: number
  maxZ: number
}

export interface CrateData {
  x: number
  z: number
  w: number
  d: number
  h: number
}

export interface SpawnData {
  team: number
  x: number
  z: number
  yaw: number
}

export interface GameMap {
  bounds: Bounds
  crates: CrateData[]
  spawns: SpawnData[]
}

export interface PlayerSnapshot {
  id: string
  name: string
  team: number
  x: number
  y: number
  z: number
  yaw: number
  pitch: number
  hp: number
  alive: boolean
  kills: number
  deaths: number
  respawnIn: number
  weapon: string
  ammo: number
  reloading: boolean
  reloadIn: number
  vest: number
  helmet: number
  ads: boolean
  assists: number
}

export interface WeaponInfo {
  id: string
  name: string
  damage: number
  headDamage: number
  fireIntervalMs: number
  magSize: number
  reloadMs: number
  spread: number
  range: number
  recoil: number
}

export interface Welcome {
  type: 'welcome'
  playerId: string
  roomId: string
  team: number
  tickRate: number
  map: GameMap
  weapons: WeaponInfo[]
}

export interface MatchState {
  state: 'waiting' | 'countdown' | 'live' | 'ended'
  score: number[]
  secondsLeft: number
  countdown: number
  winner: number
}

export interface Snapshot {
  type: 'snapshot'
  tick: number
  ack: number
  match: MatchState
  players: PlayerSnapshot[]
}

export interface Shot {
  type: 'shot'
  shooter: string
  x: number
  y: number
  z: number
  endX: number
  endY: number
  endZ: number
  hit?: string
  headshot?: boolean
}

export interface Kill {
  type: 'kill'
  killer: string
  killerName: string
  victim: string
  victimName: string
}

export interface ServerError {
  type: 'error'
  message: string
}

type Handler<T> = (msg: T) => void

export class Net {
  private ws: WebSocket | null = null
  private pingTimer: number | null = null
  private pingSentAt = 0
  private generation = 0
  private readonly name = randomName()

  latencyMs = 0

  onWelcome: Handler<Welcome> | null = null
  onSnapshot: Handler<Snapshot> | null = null
  onShot: Handler<Shot> | null = null
  onKill: Handler<Kill> | null = null
  onServerError: Handler<ServerError> | null = null
  onClose: (() => void) | null = null

  connect(weapon = 'ar'): void {
    const generation = ++this.generation
    if (this.pingTimer !== null) {
      window.clearInterval(this.pingTimer)
      this.pingTimer = null
    }
    this.ws?.close()

    const scheme = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${scheme}://${location.host}/ws`)
    this.ws = ws

    ws.onopen = () => {
      if (generation !== this.generation) return
      this.send({ type: 'join', name: this.name, weapon })
      this.pingTimer = window.setInterval(() => {
        if (generation !== this.generation) return
        this.pingSentAt = performance.now()
        this.send({ type: 'ping', t: this.pingSentAt })
      }, 5000)
    }

    ws.onmessage = (event) => {
      if (generation !== this.generation) return
      let msg: unknown
      try {
        msg = JSON.parse(String(event.data))
      } catch {
        return
      }
      if (typeof msg !== 'object' || msg === null) return
      const type = (msg as { type?: string }).type
      switch (type) {
        case 'welcome':
          this.onWelcome?.(msg as Welcome)
          break
        case 'snapshot':
          this.onSnapshot?.(msg as Snapshot)
          break
        case 'shot':
          this.onShot?.(msg as Shot)
          break
        case 'kill':
          this.onKill?.(msg as Kill)
          break
        case 'pong':
          if (this.pingSentAt > 0) {
            this.latencyMs = Math.round(performance.now() - this.pingSentAt)
          }
          break
        case 'error':
          this.onServerError?.(msg as ServerError)
          break
      }
    }

    ws.onclose = () => {
      if (generation !== this.generation) return
      if (this.pingTimer !== null) {
        window.clearInterval(this.pingTimer)
        this.pingTimer = null
      }
      this.onClose?.()
    }
  }

  send(msg: unknown): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
  }

  close(): void {
    this.ws?.close()
  }
}

function randomName(): string {
  return `Player-${Math.floor(1000 + Math.random() * 9000)}`
}
