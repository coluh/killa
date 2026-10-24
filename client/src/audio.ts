// Procedural WebAudio effects: no audio assets required.
export class Sound {
  private ctx: AudioContext | null = null
  private master: GainNode | null = null
  private volume = 0.8

  resume(): void {
    if (!this.ctx) {
      const ctor =
        window.AudioContext ??
        (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
      if (!ctor) return
      this.ctx = new ctor()
      this.master = this.ctx.createGain()
      this.master.gain.value = 0.22 * this.volume
      this.master.connect(this.ctx.destination)
    }
    if (this.ctx.state === 'suspended') {
      void this.ctx.resume()
    }
  }

  private tone(
    freq: number,
    duration: number,
    gain: number,
    type: OscillatorType,
    slideTo?: number,
  ): void {
    if (!this.ctx || !this.master) return
    const now = this.ctx.currentTime
    const osc = this.ctx.createOscillator()
    const envelope = this.ctx.createGain()
    osc.type = type
    osc.frequency.setValueAtTime(freq, now)
    if (slideTo !== undefined) {
      osc.frequency.exponentialRampToValueAtTime(slideTo, now + duration)
    }
    envelope.gain.setValueAtTime(gain, now)
    envelope.gain.exponentialRampToValueAtTime(0.0001, now + duration)
    osc.connect(envelope)
    envelope.connect(this.master)
    osc.start(now)
    osc.stop(now + duration)
  }

  private burst(duration: number, frequency: number, q: number, gain: number): void {
    if (!this.ctx || !this.master) return
    const ctx = this.ctx
    const now = ctx.currentTime
    const length = Math.max(1, Math.floor(ctx.sampleRate * duration))
    const buffer = ctx.createBuffer(1, length, ctx.sampleRate)
    const data = buffer.getChannelData(0)
    for (let i = 0; i < length; i++) {
      data[i] = (Math.random() * 2 - 1) * (1 - i / length)
    }
    const source = ctx.createBufferSource()
    source.buffer = buffer
    const filter = ctx.createBiquadFilter()
    filter.type = 'bandpass'
    filter.frequency.value = frequency
    filter.Q.value = q
    const envelope = ctx.createGain()
    envelope.gain.setValueAtTime(gain, now)
    envelope.gain.exponentialRampToValueAtTime(0.0001, now + duration)
    source.connect(filter)
    filter.connect(envelope)
    envelope.connect(this.master)
    source.start(now)
    source.stop(now + duration)
  }

  setVolume(value: number): void {
    this.volume = Math.max(0, Math.min(1, value))
    if (this.master) {
      this.master.gain.value = 0.22 * this.volume
    }
  }

  shoot(smg: boolean, volume = 1): void {
    this.resume()
    this.burst(0.08, smg ? 1900 : 950, 0.8, 0.55 * volume)
    this.tone(smg ? 190 : 150, 0.05, 0.15 * volume, 'square', smg ? 120 : 90)
  }

  hit(): void {
    this.tone(1400, 0.05, 0.2, 'triangle')
  }

  kill(): void {
    this.tone(700, 0.12, 0.18, 'sine')
    window.setTimeout(() => this.tone(350, 0.16, 0.14, 'triangle'), 70)
  }

  hurt(): void {
    this.tone(90, 0.18, 0.3, 'sine', 60)
  }

  death(): void {
    this.tone(220, 0.5, 0.25, 'sawtooth', 70)
  }

  reloadStart(): void {
    this.tone(320, 0.05, 0.12, 'square')
    window.setTimeout(() => this.tone(240, 0.05, 0.12, 'square'), 110)
  }

  reloadEnd(): void {
    this.tone(520, 0.05, 0.14, 'square')
  }

  spawn(): void {
    this.tone(300, 0.1, 0.12, 'sine', 520)
  }
}
