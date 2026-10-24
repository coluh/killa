export const PITCH_LIMIT = 1.5

export class Input {
  private readonly keys = new Set<string>()

  locked = false
  yaw = 0
  pitch = 0
  jump = false
  fire = false
  sensitivity = 0.0022

  constructor() {
    window.addEventListener('keydown', (event) => {
      if (event.code === 'Space') event.preventDefault()
      this.keys.add(event.code)
      if (event.code === 'Space') this.jump = true
    })

    window.addEventListener('keyup', (event) => {
      this.keys.delete(event.code)
    })

    document.addEventListener('mousemove', (event) => {
      if (!this.locked) return
      this.yaw -= event.movementX * this.sensitivity
      this.pitch = Math.max(
        -PITCH_LIMIT,
        Math.min(PITCH_LIMIT, this.pitch - event.movementY * this.sensitivity),
      )
    })

    document.addEventListener('mousedown', (event) => {
      if (event.button === 0 && this.locked) this.fire = true
    })

    document.addEventListener('mouseup', (event) => {
      if (event.button === 0) this.fire = false
    })

    document.addEventListener('pointerlockchange', () => {
      this.locked = document.pointerLockElement !== null
      if (!this.locked) {
        this.keys.clear()
        this.fire = false
      }
    })

    window.addEventListener('blur', () => {
      this.keys.clear()
      this.fire = false
    })
  }

  forward(): number {
    return (this.keys.has('KeyW') ? 1 : 0) - (this.keys.has('KeyS') ? 1 : 0)
  }

  right(): number {
    return (this.keys.has('KeyD') ? 1 : 0) - (this.keys.has('KeyA') ? 1 : 0)
  }

  get sprint(): boolean {
    return this.keys.has('ShiftLeft') || this.keys.has('ShiftRight')
  }

  consumeJump(): boolean {
    const jump = this.jump
    this.jump = false
    return jump
  }

  addRecoil(amount: number): void {
    this.pitch = Math.min(PITCH_LIMIT, this.pitch + amount)
  }

  setSensitivity(value: number): void {
    this.sensitivity = value
  }
}
