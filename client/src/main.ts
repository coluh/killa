import './style.css'
import { Net, type Kill, type MatchState, type PlayerSnapshot, type Shot, type Snapshot, type WeaponInfo, type Welcome } from './net'
import { World, step, type MoveInput, type MoveState } from './world'
import { Input } from './input'
import { SceneView } from './scene'
import { Sound } from './audio'

interface ProfileStats {
  kills: number
  deaths: number
  assists: number
  damage: number
  headshots: number
  matches: number
  wins: number
}

interface Profile {
  id: string
  username: string
  displayName: string
  isGuest: boolean
  createdAt: number
  loadout: string
  settings: { sens?: number; vol?: number }
  stats: ProfileStats
}

interface MatchRow {
  id: string
  roomId: string
  endedAt: string
  score: [number, number]
  winner: number
  team: number
  kills: number
  deaths: number
  assists: number
  damage: number
  headshots: number
}

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
const resultLobbyBtn = document.getElementById('resultlobby') as HTMLButtonElement
const resultTeams = document.getElementById('resultteams') as HTMLDivElement
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
const reloadTrack = document.getElementById('reloadtrack') as HTMLDivElement
const reloadFill = document.getElementById('reloadfill') as HTMLDivElement
const hurtEl = document.getElementById('hurt') as HTMLDivElement
const loadoutEl = document.getElementById('loadout') as HTMLDivElement
const loadoutCards = document.getElementById('loadoutcards') as HTMLDivElement
const countdownEl = document.getElementById('countdown') as HTMLDivElement
const sensInput = document.getElementById('sens') as HTMLInputElement
const sensValue = document.getElementById('sensval') as HTMLOutputElement
const volInput = document.getElementById('vol') as HTMLInputElement
const volValue = document.getElementById('volval') as HTMLOutputElement
const loginEl = document.getElementById('login') as HTMLDivElement
const loginForm = document.getElementById('loginform') as HTMLFormElement
const loginName = document.getElementById('loginname') as HTMLInputElement
const loginPass = document.getElementById('loginpass') as HTMLInputElement
const loginBtn = document.getElementById('loginbtn') as HTMLButtonElement
const loginError = document.getElementById('loginerror') as HTMLParagraphElement
const profileMatches = document.getElementById('profilematches') as HTMLDivElement
const guestForm = document.getElementById('guestform') as HTMLDivElement
const guestName = document.getElementById('guestname') as HTMLInputElement
const guestPass = document.getElementById('guestpass') as HTMLInputElement
const guestBtn = document.getElementById('guestbtn') as HTMLButtonElement
const passForm = document.getElementById('passform') as HTMLDivElement
const curPass = document.getElementById('curpass') as HTMLInputElement
const newPass = document.getElementById('newpass') as HTMLInputElement
const passBtn = document.getElementById('passbtn') as HTMLButtonElement
const profileMsg = document.getElementById('profilemsg') as HTMLParagraphElement
const logoutBtn = document.getElementById('logoutbtn') as HTMLButtonElement
const deleteBtn = document.getElementById('deletebtn') as HTMLButtonElement
const lobbyEl = document.getElementById('lobby') as HTMLDivElement
const lobbyName = document.getElementById('lobbyname') as HTMLHeadingElement
const lobbyTag = document.getElementById('lobbytag') as HTMLParagraphElement
const lobbyStats = document.getElementById('lobbystats') as HTMLDivElement
const lobbyWeapons = document.getElementById('lobbyweapons') as HTMLDivElement
const lobbySens = document.getElementById('lobbysens') as HTMLInputElement
const lobbySensValue = document.getElementById('lobbysensval') as HTMLOutputElement
const lobbyVol = document.getElementById('lobbyvol') as HTMLInputElement
const lobbyVolValue = document.getElementById('lobbyvolval') as HTMLOutputElement
const startMatchBtn = document.getElementById('startmatch') as HTMLButtonElement
const lobbyStatus = document.getElementById('lobbystatus') as HTMLParagraphElement

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
let profile: Profile | null = null
let authBlocked = false
let syncTimer: number | null = null
let matching = false
let resultScreen = false

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
  lobbySensValue.textContent = clamped.toFixed(2)
  lobbySens.value = String(clamped)
  saveNumber('killa.sens', clamped)
}

function applyVolume(volume: number): void {
  const clamped = Math.max(0, Math.min(1, volume))
  sound.setVolume(clamped)
  volValue.textContent = `${Math.round(clamped * 100)}%`
  volInput.value = String(clamped)
  lobbyVolValue.textContent = `${Math.round(clamped * 100)}%`
  lobbyVol.value = String(clamped)
  saveNumber('killa.vol', clamped)
}

const savedSensitivity = loadNumber('killa.sens', 1)
const savedVolume = loadNumber('killa.vol', 0.8)
applySensitivity(savedSensitivity)
applyVolume(savedVolume)
sensInput.addEventListener('input', () => {
  applySensitivity(Number(sensInput.value))
  queueProfileSync()
})
volInput.addEventListener('input', () => {
  applyVolume(Number(volInput.value))
  queueProfileSync()
})
lobbySens.addEventListener('input', () => {
  applySensitivity(Number(lobbySens.value))
  queueProfileSync()
})
lobbyVol.addEventListener('input', () => {
  applyVolume(Number(lobbyVol.value))
  queueProfileSync()
})

class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message)
  }
}

async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  const res = await fetch(path, { credentials: 'same-origin', ...init, headers })
  const text = await res.text()
  let data: any = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }
  if (!res.ok) {
    throw new ApiError(data?.error ?? `请求失败（${res.status}）`, res.status)
  }
  return data as T
}

function setProfile(next: Profile): void {
  if (syncTimer !== null) {
    window.clearTimeout(syncTimer)
    syncTimer = null
  }
  profile = next
  authBlocked = false
  loginEl.classList.add('hidden')
  if (typeof next.settings?.sens === 'number') applySensitivity(next.settings.sens)
  if (typeof next.settings?.vol === 'number') applyVolume(next.settings.vol)
  if (next.loadout === 'ar' || next.loadout === 'smg') {
    selectedLoadout = next.loadout
  }
}

function lobbyStat(label: string, value: string): HTMLDivElement {
  const box = document.createElement('div')
  const b = document.createElement('b')
  b.textContent = value
  const span = document.createElement('span')
  span.textContent = label
  box.append(b, span)
  return box
}

function renderLobby(): void {
  if (!profile) return
  lobbyName.textContent = profile.displayName
  lobbyTag.textContent = profile.isGuest
    ? '游客 · 绑定账号后可跨设备保存'
    : `账号 · ${profile.username}`

  const s = profile.stats
  const rate = s.matches > 0 ? `${Math.round((s.wins / s.matches) * 100)}%` : '—'
  lobbyStats.replaceChildren(
    lobbyStat('击杀', String(s.kills)),
    lobbyStat('胜率', rate),
    lobbyStat('场次', String(s.matches)),
  )
  renderLoadoutCards()
  lobbySens.value = sensInput.value
  lobbySensValue.textContent = sensValue.textContent
  lobbyVol.value = volInput.value
  lobbyVolValue.textContent = volValue.textContent
  guestForm.classList.toggle('hidden', !profile.isGuest)
  passForm.classList.toggle('hidden', profile.isGuest)
  startMatchBtn.disabled = false
  lobbyStatus.textContent = '4v4 · 40 杀 / 10 分钟 · 机器人补位'
}

async function refreshLobbyProfile(): Promise<void> {
  if (!profile) return
  try {
    const data = await api<{ profile: Profile; recent: MatchRow[] }>('/api/me')
    profile = data.profile
    if (!lobbyEl.classList.contains('hidden')) {
      renderLobby()
      renderMatches(data.recent ?? [])
    }
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) showLogin('登录已失效，请重新进入')
  }
}

function showLobby(): void {
  if (!profile) {
    showLogin()
    return
  }
  authBlocked = false
  matching = false
  resultScreen = false
  initialized = false
  started = false
  reconnecting = false
  if (document.pointerLockElement) document.exitPointerLock()
  lobbyEl.classList.remove('hidden')
  loginEl.classList.add('hidden')
  overlay.classList.add('hidden')
  resultOverlay.classList.add('hidden')
  deathOverlay.classList.add('hidden')
  scoreboard.classList.add('hidden')
  loadoutEl.classList.add('hidden')
  profileMsg.textContent = ''
  profileMatches.textContent = '加载中…'
  renderLobby()
  window.setTimeout(() => {
    if (!lobbyEl.classList.contains('hidden')) void refreshLobbyProfile()
  }, 250)
}

function showLogin(message = ''): void {
  if (syncTimer !== null) {
    window.clearTimeout(syncTimer)
    syncTimer = null
  }
  authBlocked = true
  matching = false
  resultScreen = false
  profile = null
  lobbyEl.classList.add('hidden')
  resultOverlay.classList.add('hidden')
  loginEl.classList.remove('hidden')
  loginError.textContent = message
  loginBtn.disabled = false
  loginBtn.textContent = '进入'
  if (document.pointerLockElement) document.exitPointerLock()
  net.close()
  window.setTimeout(() => loginName.focus(), 50)
}

function connectGame(message = '正在连接服务器…'): void {
  authBlocked = false
  matching = true
  loginEl.classList.add('hidden')
  showOverlay(message, '请稍候', true)
  net.connect(profile?.loadout || selectedLoadout)
}

async function bootstrap(): Promise<void> {
  await loadWeapons()
  try {
    const data = await api<{ profile: Profile }>('/api/session')
    setProfile(data.profile)
    showLobby()
  } catch (err) {
    const message = err instanceof ApiError && err.status === 401 ? '' : '无法连接服务器，请稍后刷新重试'
    showLogin(message)
  }
}

loginForm.addEventListener('submit', async (event) => {
  event.preventDefault()
  loginBtn.disabled = true
  loginBtn.textContent = '连接中…'
  loginError.textContent = ''
  try {
    const data = await api<{ profile: Profile }>('/api/auth', {
      method: 'POST',
      body: JSON.stringify({
        username: loginName.value.trim(),
        password: loginPass.value,
      }),
    })
    loginPass.value = ''
    setProfile(data.profile)
    showLobby()
  } catch (err) {
    loginError.textContent = err instanceof Error ? err.message : '进入失败'
    loginBtn.disabled = false
    loginBtn.textContent = '进入'
  }
})

function queueProfileSync(): void {
  if (!profile) return
  if (syncTimer !== null) window.clearTimeout(syncTimer)
  syncTimer = window.setTimeout(() => {
    syncTimer = null
    void pushProfile()
  }, 500)
}

async function pushProfile(): Promise<void> {
  if (!profile) return
  try {
    const data = await api<{ profile: Profile }>('/api/profile', {
      method: 'POST',
      body: JSON.stringify({
        settings: {
          sens: Number(sensInput.value),
          vol: Number(volInput.value),
        },
        loadout: selectedLoadout,
      }),
    })
    profile = data.profile
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) showLogin('登录已失效，请重新进入')
  }
}

function renderMatches(rows: MatchRow[]): void {
  profileMatches.replaceChildren()
  if (rows.length === 0) {
    const empty = document.createElement('p')
    empty.className = 'sub'
    empty.textContent = '还没有完成的对局'
    profileMatches.append(empty)
    return
  }
  for (const row of rows) {
    const el = document.createElement('div')
    const outcome = row.winner === 2 ? '平' : row.winner === row.team ? '胜' : '负'
    el.className = `match-row ${outcome === '胜' ? 'win' : outcome === '负' ? 'loss' : ''}`
    const when = document.createElement('span')
    const date = new Date(row.endedAt)
    when.textContent = `${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
    const score = document.createElement('b')
    score.textContent = `${row.score[0]}:${row.score[1]} ${outcome}`
    const line = document.createElement('span')
    line.textContent = `${row.kills}/${row.deaths}/${row.assists}`
    el.append(when, score, line)
    profileMatches.append(el)
  }
}

startMatchBtn.addEventListener('click', () => {
  if (!profile) return
  startMatchBtn.disabled = true
  lobbyStatus.textContent = '正在匹配…'
  lobbyEl.classList.add('hidden')
  sound.resume()
  resultScreen = false
  // Ask for pointer lock while the click still counts as a user gesture.
  requestLock(app)
  connectGame('正在匹配…')
})

resultLobbyBtn.addEventListener('click', () => {
  resultScreen = false
  resultOverlay.classList.add('hidden')
  showLobby()
})

guestBtn.addEventListener('click', async () => {
  guestBtn.disabled = true
  profileMsg.textContent = ''
  try {
    const data = await api<{ profile: Profile }>('/api/account/upgrade', {
      method: 'POST',
      body: JSON.stringify({ username: guestName.value.trim(), password: guestPass.value }),
    })
    profile = data.profile
    guestPass.value = ''
    renderLobby()
    profileMsg.style.color = 'var(--cyan)'
    profileMsg.textContent = '绑定成功，现在可以在其他设备登录'
  } catch (err) {
    profileMsg.style.color = '#ff7b72'
    profileMsg.textContent = err instanceof Error ? err.message : '绑定失败'
  } finally {
    guestBtn.disabled = false
  }
})

passBtn.addEventListener('click', async () => {
  passBtn.disabled = true
  profileMsg.textContent = ''
  try {
    await api('/api/account/password', {
      method: 'POST',
      body: JSON.stringify({ currentPassword: curPass.value, newPassword: newPass.value }),
    })
    curPass.value = ''
    newPass.value = ''
    profileMsg.style.color = 'var(--cyan)'
    profileMsg.textContent = '密码已修改，其他设备已退出'
  } catch (err) {
    profileMsg.style.color = '#ff7b72'
    profileMsg.textContent = err instanceof Error ? err.message : '修改失败'
  } finally {
    passBtn.disabled = false
  }
})

logoutBtn.addEventListener('click', async () => {
  try {
    await api('/api/auth/logout', { method: 'POST' })
  } catch {
    // ignore; clear local state anyway
  }
  showLogin('已退出登录')
})

deleteBtn.addEventListener('click', async () => {
  if (!profile) return
  if (!window.confirm('确定删除账号和所有战绩？此操作不可恢复。')) return
  deleteBtn.disabled = true
  try {
    await api('/api/account', {
      method: 'DELETE',
      body: JSON.stringify(profile.isGuest ? {} : { password: curPass.value }),
    })
    showLogin('账号已删除')
  } catch (err) {
    profileMsg.style.color = '#ff7b72'
    profileMsg.textContent = err instanceof Error ? err.message : '删除失败'
  } finally {
    deleteBtn.disabled = false
  }
})

function requestLock(target?: HTMLElement): void {
  const el = target ?? sceneView?.canvas ?? app
  const result = el.requestPointerLock() as unknown
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
  reloadTrack.classList.toggle('hidden', !me.reloading)
  input.setAdsAllowed(!me.reloading)
  if (me.reloading) {
    const remaining = Math.max(0, me.reloadIn)
    reloadHint.textContent = `换弹中 ${remaining.toFixed(1)}s`
    const total = Math.max(0.1, (myWeapon?.reloadMs ?? 1800) / 1000)
    reloadFill.style.width = `${Math.min(100, Math.max(0, (1 - remaining / total) * 100))}%`
    input.ads = false
  } else {
    reloadHint.textContent = '换弹中…'
    reloadFill.style.width = '0%'
  }
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

function selectLoadout(id: string, closeAfter: boolean): void {
  selectedLoadout = id
  saveString('killa.loadout', id)
  for (const card of document.querySelectorAll<HTMLButtonElement>('.weapon-card')) {
    card.classList.toggle('selected', card.dataset.weapon === id)
  }
  if (initialized) net.send({ type: 'loadout', weapon: id })
  queueProfileSync()
  if (closeAfter) closeLoadout(true)
}

function buildWeaponCard(info: WeaponInfo, closeAfter: boolean): HTMLButtonElement {
  const card = document.createElement('button')
  card.type = 'button'
  card.dataset.weapon = info.id
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

  card.addEventListener('click', () => selectLoadout(info.id, closeAfter))
  return card
}

function renderLoadoutCards(): void {
  loadoutCards.replaceChildren()
  lobbyWeapons.replaceChildren()
  for (const info of weapons.values()) {
    loadoutCards.append(buildWeaponCard(info, true))
    lobbyWeapons.append(buildWeaponCard(info, false))
  }
}

async function loadWeapons(): Promise<void> {
  try {
    const list = await api<WeaponInfo[]>('/api/weapons')
    weapons.clear()
    for (const info of list) weapons.set(info.id, info)
  } catch {
    // The welcome payload will fill this in after the match starts.
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
    resultNext.textContent = '点击下方按钮返回大厅'
    renderResultTeams(players)
  } else {
    resultOverlay.classList.add('hidden')
    resultTeams.replaceChildren()
  }
}

function renderResultTeams(players: PlayerSnapshot[]): void {
  const mvp = [...players].sort(
    (a, b) => b.kills - a.kills || b.assists - a.assists || a.deaths - b.deaths,
  )[0]?.id

  resultTeams.replaceChildren()
  for (const team of [0, 1]) {
    const column = document.createElement('div')
    column.className = `result-team ${team === 0 ? 'blue' : 'orange'}`
    const title = document.createElement('h4')
    title.textContent = team === 0 ? '蓝队' : '橙队'
    column.append(title)

    const list = players.filter((p) => p.team === team).sort(
      (a, b) => b.kills - a.kills || a.deaths - b.deaths || a.name.localeCompare(b.name),
    )
    for (const player of list) {
      const row = document.createElement('div')
      row.className = 'result-row'
      if (player.id === selfId) row.classList.add('self')
      if (player.id === mvp) row.classList.add('mvp')

      const name = document.createElement('span')
      name.textContent = player.id === selfId ? `${player.name}（你）` : player.name
      if (player.id === mvp) {
        const badge = document.createElement('span')
        badge.className = 'badge'
        badge.textContent = 'MVP'
        name.append(badge)
      }

      const kda = document.createElement('span')
      kda.className = 'kda'
      kda.textContent = `${player.kills}/${player.deaths}/${player.assists}`

      row.append(name, kda)
      column.append(row)
    }
    resultTeams.append(column)
  }
}

function enterResult(): void {
  if (resultScreen) return
  resultScreen = true
  matching = false
  started = false
  input.fire = false
  input.ads = false
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (document.pointerLockElement) document.exitPointerLock()
  overlay.classList.add('hidden')
  net.close()
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
  const savedLoadout = profile?.loadout || loadString('killa.loadout', 'ar')
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
  resultScreen = false
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
    if (snap.match.state === 'ended') enterResult()
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
    matching = false
    reconnecting = false
    playButton.onclick = startGame
    if (spectate) {
      overlay.classList.add('hidden')
    } else if (input.locked) {
      // The start-match click already grabbed pointer lock.
      started = true
      overlay.classList.add('hidden')
      sound.resume()
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
    const dx = shot.x - local.x
    const dz = shot.z - local.z
    const distance = Math.hypot(dx, dz)
    if (distance < 45 && distance > 0.01) {
      const rightX = Math.cos(local.yaw)
      const rightZ = -Math.sin(local.yaw)
      const pan = (dx * rightX + dz * rightZ) / distance
      sound.shoot(false, Math.max(0.08, (1 - distance / 45) * 0.5), pan)
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
  if (err.message.includes('登录')) {
    showLogin('登录已失效，请重新进入')
    return
  }
  matching = false
  reconnecting = true
  if (document.pointerLockElement) document.exitPointerLock()
  showOverlay(`无法加入：${err.message}`, '重新连接')
  playButton.onclick = () => {
    reconnectAttempt = 0
    reconnecting = true
    matching = true
    requestLock(app)
    showOverlay('正在连接服务器…', '请稍候', true)
    net.connect(selectedLoadout)
  }
}

net.onClose = () => {
  if (authBlocked || resultScreen) return
  matching = false
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
  const locked = document.pointerLockElement !== null
  if (locked) {
    if (!matching && initialized && !resultScreen) overlay.classList.add('hidden')
    sound.resume()
  } else if (started && initialized && !spectate && !loadoutOpen && !reconnecting && !resultScreen) {
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
void bootstrap()
