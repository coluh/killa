import './style.css'
import { Net, type Kill, type MatchState, type PlayerSnapshot, type Shot, type Snapshot, type WeaponInfo, type Welcome } from './net'
import { World, step, type MoveInput, type MoveState } from './world'
import { Input } from './input'
import { SceneView } from './scene'
import { Sound } from './audio'

const app = document.getElementById('app') as HTMLDivElement
const overlay = document.getElementById('overlay') as HTMLDivElement
const statusEl = document.getElementById('status') as HTMLParagraphElement
const playButton = document.getElementById('play') as HTMLButtonElement
const pingEl = document.getElementById('ping') as HTMLSpanElement
const playersEl = document.getElementById('players') as HTMLSpanElement

const hpFill = document.getElementById('hpfill') as HTMLDivElement
const hpNum = document.getElementById('hpnum') as HTMLSpanElement
const deathOverlay = document.getElementById('death') as HTMLDivElement
const deathBy = document.getElementById('deathby') as HTMLParagraphElement
const respawnEl = document.getElementById('respawnin') as HTMLParagraphElement
const killFeed = document.getElementById('killfeed') as HTMLDivElement
const crosshair = document.getElementById('crosshair') as HTMLDivElement
const adsScope = document.getElementById('ads-scope') as HTMLDivElement
const scoreAEl = document.getElementById('bigA') as HTMLSpanElement
const scoreBEl = document.getElementById('bigB') as HTMLSpanElement
const timerEl = document.getElementById('bigtimer') as HTMLDivElement
const killsEl = document.getElementById('kills') as HTMLElement
const deathsEl = document.getElementById('deaths') as HTMLElement
const assistsEl = document.getElementById('assists') as HTMLElement
const teamListEl = document.getElementById('teamlist') as HTMLDivElement
const resultOverlay = document.getElementById('result') as HTMLDivElement
const resultTitle = document.getElementById('resulttitle') as HTMLHeadingElement
const resultScore = document.getElementById('resultscore') as HTMLParagraphElement
const resultNext = document.getElementById('resultnext') as HTMLParagraphElement
const scoreboard = document.getElementById('scoreboard') as HTMLDivElement
const teamAEl = document.getElementById('teamA') as HTMLDivElement
const teamBEl = document.getElementById('teamB') as HTMLDivElement
const boardA = document.getElementById('boardA') as HTMLSpanElement
const boardB = document.getElementById('boardB') as HTMLSpanElement
const vestFill = document.getElementById('vestfill') as HTMLDivElement
const vestNum = document.getElementById('vestnum') as HTMLSpanElement
const helmetFill = document.getElementById('helmetfill') as HTMLDivElement
const helmetNum = document.getElementById('helmetnum') as HTMLSpanElement
const ammoEl = document.getElementById('ammo') as HTMLSpanElement
const magEl = document.getElementById('mag') as HTMLSpanElement
const weaponName = document.getElementById('weaponname') as HTMLDivElement
const reloadHint = document.getElementById('reloadhint') as HTMLDivElement
const hurtEl = document.getElementById('hurt') as HTMLDivElement
const loadoutEl = document.getElementById('loadout') as HTMLDivElement
const loadoutCards = document.getElementById('loadoutcards') as HTMLDivElement
const countdownEl = document.getElementById('countdown') as HTMLDivElement
const sensInput = document.getElementById('sens') as HTMLInputElement
const sensValue = document.getElementById('sensval') as HTMLOutputElement
const volInput = document.getElementById('vol') as HTMLInputElement
const volValue = document.getElementById('volval') as HTMLOutputElement

const net = new Net()
const input = new Input()
const sound = new Sound()
const spectate = new URLSearchParams(location.search).has('spectate')

let sceneView: SceneView | null = null
let world: World | null = null
let selfId = ''
let initialized = false
let started = false
let alive = true
let latestPlayers: PlayerSnapshot[] = []

let local: MoveState = { x: 0, y: 0, z: 0, vy: 0, onGround: true, yaw: 0, pitch: 0 }
let pending: Array<{ seq: number; input: MoveInput; state: MoveState }> = []
let seq = 0
let lastFrame = performance.now()
let lastShotAt = 0
let hitmarkerTimer = 0
let latestMatch: MatchState | null = null
let latestMe: PlayerSnapshot | null = null
let scoreboardVisible = false
let loadoutOpen = false
let lastHp = 100
let hurtTimer = 0
let lastReloadRequest = 0
let wasReloading = false
let reconnecting = false
let reconnectAttempt = 0
let reconnectTimer: number | null = null

const weapons = new Map<string, WeaponInfo>()
let selectedLoadout = 'ar'
let myWeapon: WeaponInfo | null = null

const BASE_SENSITIVITY = 0.0022

function loadNumber(key: string, fallback: number): number {
  try {
    const raw = localStorage.getItem(key)
    const value = raw === null ? NaN : Number(raw)
    return Number.isFinite(value) ? value : fallback
  } catch {
    return fallback
  }
}

function saveNumber(key: string, value: number): void {
  try {
    localStorage.setItem(key, String(value))
  } catch {
    // storage can be unavailable in private mode
  }
}

function loadString(key: string, fallback: string): string {
  try {
    return localStorage.getItem(key) ?? fallback
  } catch {
    return fallback
  }
}

function saveString(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    // storage can be unavailable in private mode
  }
}

function applySensitivity(multiplier: number): void {
  const clamped = Math.max(0.4, Math.min(2.2, multiplier))
  input.setSensitivity(BASE_SENSITIVITY * clamped)
  sensValue.textContent = clamped.toFixed(2)
  sensInput.value = String(clamped)
  saveNumber('killa.sens', clamped)
}

function applyVolume(volume: number): void {
  const clamped = Math.max(0, Math.min(1, volume))
  sound.setVolume(clamped)
  volValue.textContent = `${Math.round(clamped * 100)}%`
  volInput.value = String(clamped)
  saveNumber('killa.vol', clamped)
}

const savedSensitivity = loadNumber('killa.sens', 1)
const savedVolume = loadNumber('killa.vol', 0.8)
applySensitivity(savedSensitivity)
applyVolume(savedVolume)
sensInput.addEventListener('input', () => applySensitivity(Number(sensInput.value)))
volInput.addEventListener('input', () => applyVolume(Number(volInput.value)))

function requestLock(): void {
  if (!sceneView || !initialized) return
  const result = sceneView.canvas.requestPointerLock() as unknown
  if (result instanceof Promise) result.catch(() => undefined)
}

function showOverlay(message: string, buttonText: string, disabled = false): void {
  overlay.classList.remove('hidden')
  statusEl.textContent = message
  playButton.textContent = buttonText
  playButton.disabled = disabled
}

function updateVitals(me: PlayerSnapshot): void {
  const hp = Math.max(0, Math.min(100, me.hp))
  hpFill.style.width = `${hp}%`
  hpNum.textContent = String(hp)
  hpFill.classList.toggle('low', hp <= 30)

  const vest = Math.max(0, Math.min(200, me.vest))
  const helmet = Math.max(0, Math.min(200, me.helmet))
  vestFill.style.width = `${vest / 2}%`
  helmetFill.style.width = `${helmet / 2}%`
  vestNum.textContent = String(vest)
  helmetNum.textContent = String(helmet)
}

function updateWeapon(me: PlayerSnapshot): void {
  const info = weapons.get(me.weapon)
  if (info) myWeapon = info
  if (myWeapon) {
    weaponName.textContent = myWeapon.name
    magEl.textContent = String(myWeapon.magSize)
  }
  ammoEl.textContent = String(me.ammo)
  reloadHint.classList.toggle('hidden', !me.reloading)
}

function flashHurt(): void {
  hurtEl.classList.add('active')
  window.clearTimeout(hurtTimer)
  hurtTimer = window.setTimeout(() => hurtEl.classList.remove('active'), 220)
}

function requestReload(now: number): void {
  if (now - lastReloadRequest < 300) return
  lastReloadRequest = now
  net.send({ type: 'reload' })
}

function renderLoadoutCards(): void {
  loadoutCards.replaceChildren()
  for (const info of weapons.values()) {
    const card = document.createElement('button')
    card.type = 'button'
    card.className = info.id === selectedLoadout ? 'weapon-card selected' : 'weapon-card'

    const title = document.createElement('h3')
    title.textContent = info.name
    card.append(title)

    const stats = document.createElement('dl')
    const rows: Array<[string, string]> = [
      ['伤害', `${info.damage} / ${info.headDamage}`],
      ['射速', `${Math.round(60000 / info.fireIntervalMs)} RPM`],
      ['弹匣', String(info.magSize)],
      ['换弹', `${(info.reloadMs / 1000).toFixed(1)}s`],
      ['射程', `${info.range}m`],
    ]
    for (const [key, value] of rows) {
      const dt = document.createElement('dt')
      dt.textContent = key
      const dd = document.createElement('dd')
      dd.textContent = value
      stats.append(dt, dd)
    }
    card.append(stats)

    card.addEventListener('click', () => {
      selectedLoadout = info.id
      for (const child of loadoutCards.children) {
        child.classList.remove('selected')
      }
      card.classList.add('selected')
      saveString('killa.loadout', info.id)
      net.send({ type: 'loadout', weapon: info.id })
      closeLoadout(true)
    })
    loadoutCards.append(card)
  }
}

function openLoadout(): void {
  if (!initialized || spectate) return
  loadoutOpen = true
  renderLoadoutCards()
  loadoutEl.classList.remove('hidden')
  if (document.pointerLockElement) document.exitPointerLock()
}

function closeLoadout(relock: boolean): void {
  loadoutOpen = false
  loadoutEl.classList.add('hidden')
  if (!relock) return
  started = true
  requestLock()
  window.setTimeout(() => {
    if (!document.pointerLockElement && initialized && !spectate) {
      showOverlay('已暂停', '继续战斗')
    }
  }, 350)
}

function showDeath(reason: string): void {
  deathOverlay.classList.remove('hidden')
  if (reason) deathBy.textContent = reason
}

function hideDeath(): void {
  deathOverlay.classList.add('hidden')
  deathBy.textContent = ''
}

function flashHitmarker(): void {
  crosshair.classList.add('hit')
  window.clearTimeout(hitmarkerTimer)
  hitmarkerTimer = window.setTimeout(() => crosshair.classList.remove('hit'), 120)
}

function addKillFeed(kill: Kill): void {
  const item = document.createElement('div')
  item.className = 'feed-item'
  item.textContent = `${kill.killerName}  ▸  ${kill.victimName}`
  killFeed.prepend(item)
  while (killFeed.children.length > 5) {
    killFeed.lastElementChild?.remove()
  }
  window.setTimeout(() => item.remove(), 5000)
}

function formatClock(seconds: number): string {
  const total = Math.max(0, Math.ceil(seconds))
  const minutes = Math.floor(total / 60)
  const rest = total % 60
  return `${minutes}:${String(rest).padStart(2, '0')}`
}

function updateMatchHud(match: MatchState, players: PlayerSnapshot[]): void {
  if (match.state === 'countdown') {
    countdownEl.textContent = String(Math.max(1, Math.ceil(match.countdown)))
    countdownEl.classList.remove('hidden')
  } else {
    countdownEl.classList.add('hidden')
  }

  scoreAEl.textContent = String(match.score[0] ?? 0)
  scoreBEl.textContent = String(match.score[1] ?? 0)

  switch (match.state) {
    case 'waiting':
      timerEl.textContent = `等待玩家 ${players.length}/2`
      break
    case 'countdown':
      timerEl.textContent = `即将开始 ${Math.max(0, Math.ceil(match.countdown))}s`
      break
    case 'live':
      timerEl.textContent = formatClock(match.secondsLeft)
      break
    case 'ended':
      timerEl.textContent = '结算中'
      break
  }

  if (match.state === 'ended') {
    resultOverlay.classList.remove('hidden')
    resultTitle.textContent =
      match.winner === 0 ? '蓝队胜利' : match.winner === 1 ? '橙队胜利' : match.winner === 2 ? '平局' : '对局结束'
    resultTitle.className = match.winner === 0 ? 'blue' : match.winner === 1 ? 'orange' : ''
    resultScore.textContent = `${match.score[0] ?? 0} : ${match.score[1] ?? 0}`
    resultNext.textContent = `${Math.max(0, Math.ceil(match.countdown))}s 后开始新对局`
  } else {
    resultOverlay.classList.add('hidden')
  }
}

function renderTeamRows(el: HTMLDivElement, list: PlayerSnapshot[]): void {
  el.replaceChildren()

  const head = document.createElement('div')
  head.className = 'row head'
  head.innerHTML = '<span>玩家</span><span>K</span><span>D</span>'
  el.append(head)

  const sorted = [...list].sort(
    (a, b) => b.kills - a.kills || a.deaths - b.deaths || a.name.localeCompare(b.name),
  )
  for (const player of sorted) {
    const row = document.createElement('div')
    row.className = player.id === selfId ? 'row self' : 'row'

    const name = document.createElement('span')
    name.className = 'pname'
    name.textContent = player.name

    const kills = document.createElement('span')
    kills.className = 'pk'
    kills.textContent = String(player.kills)

    const deaths = document.createElement('span')
    deaths.className = 'pd'
    deaths.textContent = String(player.deaths)

    const assists = document.createElement('span')
    assists.className = 'pa'
    assists.textContent = String(player.assists)

    row.append(name, kills, deaths, assists)
    el.append(row)
  }

  if (sorted.length === 0) {
    const empty = document.createElement('div')
    empty.className = 'row empty'
    empty.textContent = '—'
    el.append(empty)
  }
}

function renderTeamList(players: PlayerSnapshot[], me: PlayerSnapshot): void {
  teamListEl.replaceChildren()
  for (const mate of players) {
    if (mate.team !== me.team || mate.id === me.id) continue

    const row = document.createElement('div')
    row.className = mate.alive ? 'mate' : 'mate dead'

    const name = document.createElement('span')
    name.className = 'mate-name'
    name.textContent = mate.name

    const track = document.createElement('div')
    track.className = 'mate-track'
    const fill = document.createElement('div')
    fill.className = 'mate-fill'
    fill.style.width = `${Math.max(0, Math.min(100, mate.hp))}%`
    track.append(fill)

    const hp = document.createElement('span')
    hp.className = 'mate-hp'
    hp.textContent = mate.alive ? String(mate.hp) : '—'

    row.append(name, track, hp)
    teamListEl.append(row)
  }
}

function renderScoreboard(players: PlayerSnapshot[], match: MatchState): void {
  boardA.textContent = String(match.score[0] ?? 0)
  boardB.textContent = String(match.score[1] ?? 0)
  renderTeamRows(teamAEl, players.filter((p) => p.team === 0))
  renderTeamRows(teamBEl, players.filter((p) => p.team === 1))
}

net.onWelcome = (welcome: Welcome) => {
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  reconnectAttempt = 0

  selfId = welcome.playerId
  world = new World(welcome.map)
  sceneView?.dispose()
  sceneView = new SceneView(app, welcome.map, welcome.team)

  weapons.clear()
  for (const info of welcome.weapons) {
    weapons.set(info.id, info)
  }
  const savedLoadout = loadString('killa.loadout', 'ar')
  selectedLoadout = weapons.has(savedLoadout) ? savedLoadout : (welcome.weapons[0]?.id ?? 'ar')
  myWeapon = weapons.get(selectedLoadout) ?? null
  renderLoadoutCards()

  initialized = false
  alive = true
  latestPlayers = []
  latestMe = null
  latestMatch = null
  pending = []
  seq = 0
  wasReloading = false
  lastHp = 100
  lastShotAt = 0
  loadoutOpen = false
  scoreboardVisible = false
  hideDeath()
  deathOverlay.classList.add('hidden')
  resultOverlay.classList.add('hidden')
  loadoutEl.classList.add('hidden')
  scoreboard.classList.add('hidden')
  playButton.onclick = startGame

  const spawn = welcome.map.spawns.find((s) => s.team === welcome.team)
  if (spawn) {
    local = { x: spawn.x, y: 0, z: spawn.z, vy: 0, onGround: true, yaw: spawn.yaw, pitch: 0 }
    input.yaw = spawn.yaw
    input.pitch = 0
  }

  statusEl.textContent = '已加入房间，正在同步…'
}

net.onSnapshot = (snap: Snapshot) => {
  latestPlayers = snap.players
  latestMatch = snap.match ?? null
  playersEl.textContent = `${snap.players.length}/8`
  if (snap.match) {
    updateMatchHud(snap.match, snap.players)
    if (scoreboardVisible) renderScoreboard(snap.players, snap.match)
  }

  const me = snap.players.find((p) => p.id === selfId)
  if (!me || !world) return

  latestMe = me
  updateVitals(me)
  updateWeapon(me)
  killsEl.textContent = String(me.kills)
  deathsEl.textContent = String(me.deaths)
  assistsEl.textContent = String(me.assists)
  renderTeamList(snap.players, me)
  if (me.hp < lastHp) {
    flashHurt()
    sound.hurt()
  }
  lastHp = me.hp

  if (me.reloading && !wasReloading) sound.reloadStart()
  if (!me.reloading && wasReloading) sound.reloadEnd()
  wasReloading = me.reloading

  if (!initialized) {
    local = { x: me.x, y: me.y, z: me.z, vy: 0, onGround: me.y <= 0.001, yaw: local.yaw, pitch: local.pitch }
    initialized = true
    reconnecting = false
    playButton.onclick = startGame
    if (spectate) {
      overlay.classList.add('hidden')
    } else {
      showOverlay('连接成功，点击进入战场', '进入战场')
    }
    return
  }

  if (!me.alive) {
    alive = false
    input.fire = false
    input.ads = false
    showDeath('')
    respawnEl.textContent = `${Math.max(0, me.respawnIn).toFixed(1)}s 后复活`
    return
  }

  if (!alive) {
    alive = true
    hideDeath()
    sound.spawn()
    pending = []
    lastShotAt = 0
    local = { x: me.x, y: me.y, z: me.z, vy: 0, onGround: true, yaw: me.yaw, pitch: me.pitch }
    input.yaw = me.yaw
    input.pitch = me.pitch
    return
  }

  const ack = snap.ack
  const index = pending.findIndex((p) => p.seq === ack)
  if (index === -1) {
    pending = pending.filter((p) => p.seq > ack)
    return
  }

  const predicted = pending[index].state
  const error = Math.hypot(me.x - predicted.x, me.y - predicted.y, me.z - predicted.z)
  const remaining = pending.slice(index + 1)
  pending = remaining

  if (error > 0.75) {
    let corrected: MoveState = {
      x: me.x,
      y: me.y,
      z: me.z,
      vy: 0,
      onGround: me.y <= 0.001,
      yaw: local.yaw,
      pitch: local.pitch,
    }
    for (const item of remaining) {
      corrected = step(corrected, item.input, world)
      item.state = corrected
    }
    local = corrected
  }
}

net.onShot = (shot: Shot) => {
  if (!sceneView) return
  if (shot.shooter === selfId) {
    const muzzle = sceneView.muzzlePosition()
    sceneView.addTracer({ ...shot, x: muzzle.x, y: muzzle.y, z: muzzle.z }, true)
  } else {
    sceneView.addTracer(shot, false)
  }
  if (shot.shooter === selfId) {
    sound.shoot(myWeapon?.id === 'smg', 0.9)
  } else {
    const distance = Math.hypot(shot.x - local.x, shot.z - local.z)
    if (distance < 45) {
      sound.shoot(false, Math.max(0.08, (1 - distance / 45) * 0.5))
    }
  }
  sceneView.addImpact(shot.endX, shot.endY, shot.endZ, Boolean(shot.hit))
  if (shot.hit && shot.shooter === selfId) {
    flashHitmarker()
    sound.hit()
  }
}

net.onKill = (kill: Kill) => {
  addKillFeed(kill)
  if (kill.killer === selfId) sound.kill()
  if (kill.victim === selfId) {
    sound.death()
    alive = false
    input.fire = false
    input.ads = false
    showDeath(`被 ${kill.killerName} 击杀`)
    respawnEl.textContent = '3.0s 后复活'
  }
}

net.onServerError = (err) => {
  reconnecting = true
  showOverlay(`无法加入：${err.message}`, '重新连接')
  playButton.onclick = () => {
    reconnectAttempt = 0
    reconnecting = true
    showOverlay('正在连接服务器…', '请稍候', true)
    net.connect(selectedLoadout)
  }
}

net.onClose = () => {
  reconnecting = true
  if (document.pointerLockElement) document.exitPointerLock()
  scheduleReconnect()
}

function scheduleReconnect(): void {
  if (reconnectTimer !== null) return

  if (reconnectAttempt >= 5) {
    showOverlay('无法连接服务器', '重新连接')
    playButton.onclick = () => {
      reconnectAttempt = 0
      reconnecting = true
      showOverlay('正在连接服务器…', '请稍候', true)
      net.connect(selectedLoadout)
    }
    return
  }

  reconnectAttempt++
  showOverlay(`连接断开，正在重连 ${reconnectAttempt}/5…`, '请稍候', true)
  const delay = Math.min(5000, 700 * reconnectAttempt)
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    net.connect(selectedLoadout)
  }, delay)
}

function startGame(): void {
  sound.resume()
  if (spectate) {
    overlay.classList.add('hidden')
    return
  }
  started = true
  requestLock()
}

playButton.onclick = startGame

document.addEventListener('pointerlockchange', () => {
  const locked = document.pointerLockElement === sceneView?.canvas
  if (locked) {
    overlay.classList.add('hidden')
    sound.resume()
  } else if (started && initialized && !spectate && !loadoutOpen && !reconnecting) {
    showOverlay('已暂停', '继续战斗')
  }
})

window.addEventListener('keydown', (event) => {
  if (event.code !== 'Tab') return
  event.preventDefault()
  if (!initialized) return
  scoreboardVisible = true
  scoreboard.classList.remove('hidden')
  if (latestMatch) renderScoreboard(latestPlayers, latestMatch)
})

window.addEventListener('keyup', (event) => {
  if (event.code !== 'Tab') return
  scoreboardVisible = false
  scoreboard.classList.add('hidden')
})

window.addEventListener('keydown', (event) => {
  if (event.code === 'KeyB') {
    event.preventDefault()
    if (loadoutOpen) {
      closeLoadout(true)
    } else {
      openLoadout()
    }
    return
  }
  if (event.code === 'KeyR') {
    event.preventDefault()
    if (alive && input.locked) requestReload(performance.now())
    return
  }
  if (event.code === 'Escape' && loadoutOpen) {
    closeLoadout(false)
    showOverlay('已暂停', '继续战斗')
  }
})

window.addEventListener('blur', () => {
  scoreboardVisible = false
  scoreboard.classList.add('hidden')
})

window.setInterval(() => {
  pingEl.textContent = net.latencyMs > 0 ? `${net.latencyMs} ms` : '— ms'
}, 500)

function frame(now: number): void {
  requestAnimationFrame(frame)

  const dt = Math.min((now - lastFrame) / 1000, 0.05)
  lastFrame = now

  if (initialized && input.locked && world && alive) {
    const move: MoveInput = {
      dt,
      forward: input.forward(),
      right: input.right(),
      sprint: input.sprint,
      jump: input.consumeJump(),
      yaw: input.yaw,
      pitch: input.pitch,
      ads: input.ads,
    }

    seq += 1
    local = step(local, move, world)
    pending.push({ seq, input: move, state: local })
    net.send({ type: 'input', seq, ...move, rtt: net.latencyMs })

    if (input.fire && myWeapon) {
      if (!latestMe || latestMe.ammo <= 0 || latestMe.reloading) {
        requestReload(now)
      } else if (now - lastShotAt >= myWeapon.fireIntervalMs + 10) {
        lastShotAt = now
        net.send({ type: 'shoot', yaw: input.yaw, pitch: input.pitch })
        input.addRecoil(myWeapon.recoil)
      }
    }
  }

  if (sceneView) {
    sceneView.update(dt, local, latestPlayers, selfId, input.ads, input.sprint && input.forward() > 0)
  }
  crosshair.classList.toggle('ads', input.ads)
  adsScope.classList.toggle('hidden', !input.ads)
}

if (new URLSearchParams(location.search).has('debug')) {
  ;(window as unknown as { __killa?: { net: Net } }).__killa = { net }
}

requestAnimationFrame(frame)
net.connect(loadString('killa.loadout', 'ar'))
