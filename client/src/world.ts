import type { GameMap } from './net'

export const PLAYER_RADIUS = 0.4
export const EYE_HEIGHT = 1.6
export const WALK_SPEED = 4.8
export const SPRINT_SPEED = 6.6
export const GRAVITY = 20
export const JUMP_SPEED = 7.2
export const MAX_DT = 0.1
export const PITCH_LIMIT = 1.5

export interface MoveState {
  x: number
  y: number
  z: number
  vy: number
  onGround: boolean
  yaw: number
  pitch: number
}

export interface MoveInput {
  dt: number
  forward: number
  right: number
  sprint: boolean
  jump: boolean
  yaw: number
  pitch: number
}

function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v
}

export class World {
  readonly map: GameMap

  constructor(map: GameMap) {
    this.map = map
  }

  blocked(x: number, z: number, radius: number): boolean {
    return this.blockedAt(x, z, radius, 0)
  }

  blockedAt(x: number, z: number, radius: number, feetY: number): boolean {
    const r2 = radius * radius
    for (const c of this.map.crates) {
      if (feetY + 0.05 >= c.h) continue
      const minX = c.x - c.w / 2
      const maxX = c.x + c.w / 2
      const minZ = c.z - c.d / 2
      const maxZ = c.z + c.d / 2
      const cx = clamp(x, minX, maxX)
      const cz = clamp(z, minZ, maxZ)
      const dx = x - cx
      const dz = z - cz
      if (dx * dx + dz * dz < r2) return true
    }
    return false
  }

  supportHeight(x: number, z: number, radius: number, feetY: number): number {
    let best = 0
    const r2 = radius * radius
    for (const c of this.map.crates) {
      if (c.h > feetY + 0.05) continue
      const minX = c.x - c.w / 2
      const maxX = c.x + c.w / 2
      const minZ = c.z - c.d / 2
      const maxZ = c.z + c.d / 2
      const cx = clamp(x, minX, maxX)
      const cz = clamp(z, minZ, maxZ)
      const dx = x - cx
      const dz = z - cz
      if (dx * dx + dz * dz < r2 && c.h > best) best = c.h
    }
    return best
  }

  boundsXZ(x: number, z: number, radius: number): { x: number; z: number } {
    const b = this.map.bounds
    return {
      x: clamp(x, b.minX + radius, b.maxX - radius),
      z: clamp(z, b.minZ + radius, b.maxZ - radius),
    }
  }
}

/**
 * 与服务端 `game.Step` 保持完全一致的移动模拟。
 * 任何改动必须同步 server/internal/game/movement.go。
 */
export function step(state: MoveState, input: MoveInput, world: World): MoveState {
  const dt = clamp(input.dt, 0, MAX_DT)

  const s: MoveState = { ...state }
  s.yaw = input.yaw
  s.pitch = clamp(input.pitch, -PITCH_LIMIT, PITCH_LIMIT)

  let forward = input.forward
  let right = input.right
  const magnitude = Math.hypot(forward, right)
  if (magnitude > 1) {
    forward /= magnitude
    right /= magnitude
  }

  const speed = input.sprint ? SPRINT_SPEED : WALK_SPEED
  const sin = Math.sin(s.yaw)
  const cos = Math.cos(s.yaw)
  const dx = (-forward * sin + right * cos) * speed * dt
  const dz = (-forward * cos - right * sin) * speed * dt

  if (!world.blockedAt(s.x + dx, s.z, PLAYER_RADIUS, s.y)) s.x += dx
  if (!world.blockedAt(s.x, s.z + dz, PLAYER_RADIUS, s.y)) s.z += dz

  const bounded = world.boundsXZ(s.x, s.z, PLAYER_RADIUS)
  s.x = bounded.x
  s.z = bounded.z

  if (input.jump && s.onGround) {
    s.vy = JUMP_SPEED
    s.onGround = false
  }
  const prevY = s.y
  s.vy -= GRAVITY * dt
  s.y += s.vy * dt

  // Use the pre-gravity height so a fast fall cannot tunnel through a crate top.
  const support = world.supportHeight(s.x, s.z, PLAYER_RADIUS, prevY)
  if (s.y <= support) {
    s.y = support
    s.vy = 0
    s.onGround = true
  } else {
    s.onGround = false
  }

  return s
}
