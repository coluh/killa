import * as THREE from 'three'
import type { GameMap, PlayerSnapshot, Shot } from './net'
import { EYE_HEIGHT, type MoveState } from './world'

interface Remote {
  group: THREE.Group
  pos: THREE.Vector3
  target: THREE.Vector3
  yaw: number
  targetYaw: number
  alive: boolean
}

interface Tracer {
  line: THREE.Line
  material: THREE.LineBasicMaterial
  life: number
  maxLife: number
}

function shortestAngle(from: number, to: number): number {
  const twoPi = Math.PI * 2
  let delta = (to - from) % twoPi
  if (delta > Math.PI) delta -= twoPi
  if (delta < -Math.PI) delta += twoPi
  return delta
}

function makePlayer(team: number): THREE.Group {
  const group = new THREE.Group()
  const color = team === 0 ? 0x3fa7ff : 0xff7043

  const body = new THREE.Mesh(
    new THREE.BoxGeometry(0.55, 1.15, 0.38),
    new THREE.MeshStandardMaterial({
      color,
      roughness: 0.55,
      metalness: 0.25,
      emissive: color,
      emissiveIntensity: 0.12,
    }),
  )
  body.position.y = 0.62
  body.castShadow = true
  group.add(body)

  const head = new THREE.Mesh(
    new THREE.BoxGeometry(0.3, 0.3, 0.3),
    new THREE.MeshStandardMaterial({ color: 0xcfd8e3, roughness: 0.8, metalness: 0.1 }),
  )
  head.position.y = 1.39
  head.castShadow = true
  group.add(head)

  const gun = new THREE.Mesh(
    new THREE.BoxGeometry(0.08, 0.08, 0.5),
    new THREE.MeshStandardMaterial({ color: 0x1b1f27, metalness: 0.8, roughness: 0.35 }),
  )
  gun.position.set(0.17, 1.03, -0.3)
  group.add(gun)

  const ring = new THREE.Mesh(
    new THREE.RingGeometry(0.42, 0.5, 32),
    new THREE.MeshBasicMaterial({
      color,
      transparent: true,
      opacity: 0.55,
      side: THREE.DoubleSide,
      depthWrite: false,
    }),
  )
  ring.rotation.x = -Math.PI / 2
  ring.position.y = 0.02
  group.add(ring)

  return group
}

export class SceneView {
  readonly canvas: HTMLCanvasElement

  private readonly scene = new THREE.Scene()
  private readonly camera: THREE.PerspectiveCamera
  private readonly renderer: THREE.WebGLRenderer
  private readonly remotes = new Map<string, Remote>()
  private readonly tracers: Tracer[] = []
  private readonly handleResize = (): void => this.resize()
  private viewmodel: THREE.Group | null = null

  constructor(container: HTMLElement, map: GameMap, selfTeam: number) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: 'high-performance' })
    this.canvas = this.renderer.domElement
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    this.renderer.setSize(window.innerWidth, window.innerHeight)
    this.renderer.shadowMap.enabled = true
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap
    this.renderer.outputColorSpace = THREE.SRGBColorSpace
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping
    this.renderer.toneMappingExposure = 1.15
    container.appendChild(this.canvas)

    this.scene.background = new THREE.Color(0x0b0e14)
    this.scene.fog = new THREE.Fog(0x0b0e14, 38, 80)

    this.camera = new THREE.PerspectiveCamera(75, window.innerWidth / window.innerHeight, 0.05, 200)
    this.camera.rotation.order = 'YXZ'
    this.scene.add(this.camera)

    this.buildLights()
    this.buildMap(map)
    this.buildViewmodel(selfTeam)

    window.addEventListener('resize', this.handleResize)
  }

  private buildLights(): void {
    this.scene.add(new THREE.HemisphereLight(0x9fb8ff, 0x0a0c10, 1.2))

    const sun = new THREE.DirectionalLight(0xffffff, 2.4)
    sun.position.set(14, 26, 10)
    sun.castShadow = true
    sun.shadow.mapSize.set(1024, 1024)
    const shadow = sun.shadow.camera
    shadow.left = -30
    shadow.right = 30
    shadow.top = 30
    shadow.bottom = -30
    shadow.near = 1
    shadow.far = 80
    this.scene.add(sun)

    const fill = new THREE.DirectionalLight(0x4cc9f0, 0.5)
    fill.position.set(-12, 8, -14)
    this.scene.add(fill)
  }

  private buildMap(map: GameMap): void {
    const b = map.bounds
    const width = b.maxX - b.minX
    const depth = b.maxZ - b.minZ
    const cx = (b.minX + b.maxX) / 2
    const cz = (b.minZ + b.maxZ) / 2

    const floor = new THREE.Mesh(
      new THREE.PlaneGeometry(width + 4, depth + 4),
      new THREE.MeshStandardMaterial({ color: 0x141821, roughness: 0.95, metalness: 0.05 }),
    )
    floor.rotation.x = -Math.PI / 2
    floor.position.set(cx, 0, cz)
    floor.receiveShadow = true
    this.scene.add(floor)

    const gridSize = Math.max(width, depth)
    const grid = new THREE.GridHelper(gridSize, Math.round(gridSize), 0x33465e, 0x1b2533)
    grid.position.set(cx, 0.01, cz)
    this.scene.add(grid)

    map.crates.forEach((crate, index) => {
      const geometry = new THREE.BoxGeometry(crate.w, crate.h, crate.d)
      const shade = 0.16 + (index % 3) * 0.025
      const mesh = new THREE.Mesh(
        geometry,
        new THREE.MeshStandardMaterial({
          color: new THREE.Color().setHSL(0.58, 0.1, shade),
          roughness: 0.85,
          metalness: 0.15,
        }),
      )
      mesh.position.set(crate.x, crate.h / 2, crate.z)
      mesh.castShadow = true
      mesh.receiveShadow = true
      this.scene.add(mesh)

      const edges = new THREE.LineSegments(
        new THREE.EdgesGeometry(geometry),
        new THREE.LineBasicMaterial({ color: 0x59d8ff, transparent: true, opacity: 0.7 }),
      )
      edges.position.copy(mesh.position)
      edges.scale.setScalar(1.003) // 避免与箱体表面 z-fighting
      this.scene.add(edges)
    })

    for (const spawn of map.spawns) {
      const padColor = spawn.team === 0 ? 0x4cc9f0 : 0xff7043
      const pad = new THREE.Mesh(
        new THREE.RingGeometry(0.5, 0.68, 28),
        new THREE.MeshBasicMaterial({
          color: padColor,
          transparent: true,
          opacity: 0.32,
          side: THREE.DoubleSide,
          depthWrite: false,
        }),
      )
      pad.rotation.x = -Math.PI / 2
      pad.position.set(spawn.x, 0.02, spawn.z)
      this.scene.add(pad)
    }

    const beamMaterial = new THREE.MeshStandardMaterial({ color: 0x1b2330, roughness: 0.9 })
    for (const offset of [-12, -4, 4, 12]) {
      const beam = new THREE.Mesh(new THREE.BoxGeometry(width + 2, 0.35, 0.5), beamMaterial)
      beam.position.set(cx, 6, cz + offset)
      this.scene.add(beam)
    }

    const wallMaterial = new THREE.MeshStandardMaterial({ color: 0x0f141c, roughness: 1 })
    const wallHeight = 5
    const thickness = 0.6
    const walls: Array<[number, number, number, number]> = [
      [cx, b.minZ - thickness / 2, width + thickness * 2, thickness],
      [cx, b.maxZ + thickness / 2, width + thickness * 2, thickness],
      [b.minX - thickness / 2, cz, thickness, depth],
      [b.maxX + thickness / 2, cz, thickness, depth],
    ]
    for (const [x, z, w, d] of walls) {
      const wall = new THREE.Mesh(new THREE.BoxGeometry(w, wallHeight, d), wallMaterial)
      wall.position.set(x, wallHeight / 2, z)
      wall.receiveShadow = true
      this.scene.add(wall)
    }
  }

  private buildViewmodel(team: number): void {
    const group = new THREE.Group()

    const metal = new THREE.MeshStandardMaterial({ color: 0x4b586b, metalness: 0.2, roughness: 0.55 })
    const dark = new THREE.MeshStandardMaterial({ color: 0x2b3340, metalness: 0.2, roughness: 0.7 })
    const accentColor = team === 0 ? 0x4cc9f0 : 0xff7043
    const accent = new THREE.MeshStandardMaterial({
      color: accentColor,
      emissive: accentColor,
      emissiveIntensity: 0.9,
      roughness: 0.4,
    })

    const glove = new THREE.MeshStandardMaterial({ color: 0x1e242e, metalness: 0.1, roughness: 0.9 })

    const parts: Array<[THREE.BoxGeometry, THREE.Material, [number, number, number]]> = [
      [new THREE.BoxGeometry(0.09, 0.11, 0.42), metal, [0.24, -0.2, -0.85]],
      [new THREE.BoxGeometry(0.04, 0.04, 0.36), dark, [0.24, -0.185, -1.22]],
      [new THREE.BoxGeometry(0.06, 0.16, 0.09), dark, [0.24, -0.31, -0.82]],
      [new THREE.BoxGeometry(0.07, 0.09, 0.18), dark, [0.24, -0.21, -0.62]],
      [new THREE.BoxGeometry(0.03, 0.045, 0.08), accent, [0.24, -0.13, -0.86]],
      [new THREE.BoxGeometry(0.1, 0.12, 0.12), glove, [0.24, -0.3, -0.78]],
    ]

    for (const [geometry, material, position] of parts) {
      const mesh = new THREE.Mesh(geometry, material)
      mesh.position.set(position[0], position[1], position[2])
      group.add(mesh)
    }

    const keyLight = new THREE.PointLight(0xbfe8ff, 4, 3, 2)
    keyLight.position.set(0.3, 0.1, -0.2)
    this.camera.add(keyLight)

    this.viewmodel = group
    this.camera.add(group)
  }

  muzzlePosition(): THREE.Vector3 {
    return new THREE.Vector3(0.26, -0.18, -1.3)
      .applyEuler(this.camera.rotation)
      .add(this.camera.position)
  }

  addTracer(shot: Shot, own: boolean): void {
    const from = new THREE.Vector3(shot.x, shot.y, shot.z)
    const to = new THREE.Vector3(shot.endX, shot.endY, shot.endZ)
    const direction = to.clone().sub(from)
    if (direction.lengthSq() < 0.01) return
    direction.normalize()
    from.addScaledVector(direction, 0.4)

    const geometry = new THREE.BufferGeometry().setFromPoints([from, to])
    const material = new THREE.LineBasicMaterial({
      color: own ? 0x9ff2ff : 0xffb27a,
      transparent: true,
      opacity: 0.85,
      depthWrite: false,
    })
    const line = new THREE.Line(geometry, material)
    this.scene.add(line)
    this.tracers.push({ line, material, life: 0.12, maxLife: 0.12 })
  }

  dispose(): void {
    window.removeEventListener('resize', this.handleResize)
    this.renderer.dispose()
    this.canvas.remove()
  }

  private resize(): void {
    this.camera.aspect = window.innerWidth / window.innerHeight
    this.camera.updateProjectionMatrix()
    this.renderer.setSize(window.innerWidth, window.innerHeight)
  }

  update(
    dt: number,
    local: MoveState,
    players: PlayerSnapshot[],
    selfId: string,
    ads: boolean,
    sprinting: boolean,
  ): void {
    this.camera.position.set(local.x, local.y + EYE_HEIGHT, local.z)
    this.camera.rotation.set(local.pitch, local.yaw, 0)

    const targetFov = ads ? 55 : sprinting ? 80 : 75
    this.camera.fov += (targetFov - this.camera.fov) * (1 - Math.exp(-14 * dt))
    this.camera.updateProjectionMatrix()

    if (this.viewmodel) {
      const blend = 1 - Math.exp(-16 * dt)
      const targetX = ads ? -0.24 : 0
      const targetY = ads ? 0.07 : 0
      const targetZ = ads ? 0.12 : 0
      this.viewmodel.position.x += (targetX - this.viewmodel.position.x) * blend
      this.viewmodel.position.y += (targetY - this.viewmodel.position.y) * blend
      this.viewmodel.position.z += (targetZ - this.viewmodel.position.z) * blend
    }

    const seen = new Set<string>()
    for (const player of players) {
      if (player.id === selfId) continue
      seen.add(player.id)

      let remote = this.remotes.get(player.id)
      if (!remote) {
        remote = {
          group: makePlayer(player.team),
          pos: new THREE.Vector3(player.x, player.y, player.z),
          target: new THREE.Vector3(player.x, player.y, player.z),
          yaw: player.yaw,
          targetYaw: player.yaw,
          alive: player.alive,
        }
        remote.group.visible = player.alive
        this.scene.add(remote.group)
        this.remotes.set(player.id, remote)
      }

      if (player.alive && !remote.alive) {
        remote.pos.set(player.x, player.y, player.z)
        remote.target.set(player.x, player.y, player.z)
        remote.yaw = player.yaw
        remote.targetYaw = player.yaw
      }
      remote.alive = player.alive
      remote.group.visible = player.alive
      if (player.alive) {
        remote.target.set(player.x, player.y, player.z)
        remote.targetYaw = player.yaw
      }
    }

    for (const [id, remote] of this.remotes) {
      if (seen.has(id)) continue
      this.scene.remove(remote.group)
      this.remotes.delete(id)
    }

    const smoothing = 1 - Math.exp(-14 * dt)
    for (const remote of this.remotes.values()) {
      remote.pos.lerp(remote.target, smoothing)
      remote.yaw += shortestAngle(remote.yaw, remote.targetYaw) * smoothing
      remote.group.position.copy(remote.pos)
      remote.group.rotation.y = remote.yaw
    }

    for (let i = this.tracers.length - 1; i >= 0; i--) {
      const tracer = this.tracers[i]
      tracer.life -= dt
      if (tracer.life <= 0) {
        this.scene.remove(tracer.line)
        tracer.line.geometry.dispose()
        tracer.material.dispose()
        this.tracers.splice(i, 1)
      } else {
        tracer.material.opacity = 0.85 * (tracer.life / tracer.maxLife)
      }
    }

    this.renderer.render(this.scene, this.camera)
  }
}
