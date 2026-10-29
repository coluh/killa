// Procedural WebAudio effects: no audio assets required.
export class Sound {
  private ctx: AudioContext | null = null
  private master: GainNode | null = null
  private compressor: DynamicsCompressorNode | null = null
  private volume = 0.8

  resume(): void {
    if (!this.ctx) {
      const ctor =
        window.AudioContext ??
        (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
      if (!ctor) return
      this.ctx = new ctor()
      this.master = this.ctx.createGain()
      this.master.gain.value = 0.5 * this.volume
      this.compressor = this.ctx.createDynamicsCompressor()
      this.compressor.threshold.value = -16
      this.compressor.knee.value = 24
      this.compressor.ratio.value = 6
      this.compressor.attack.value = 0.003
      this.compressor.release.value = 0.25
      this.master.connect(this.compressor)
      this.compressor.connect(this.ctx.destination)
    }
    if (this.ctx.state === 'suspended') {
      void this.ctx.resume()
    }
  }

  private output(node: AudioNode, pan: number): void {
    if (!this.master || !this.ctx) return
    if (pan !== 0 && typeof this.ctx.createStereoPanner === 'function') {
      const panner = this.ctx.createStereoPanner()
      panner.pan.value = Math.max(-1, Math.min(1, pan))
      node.connect(panner)
      panner.connect(this.master)
    } else {
      node.connect(this.master)
    }
  }

  private tone(
    freq: number,
    duration: number,
    gain: number,
    type: OscillatorType,
    slideTo?: number,
    pan = 0,
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
    this.output(envelope, pan)
    osc.start(now)
    osc.stop(now + duration)
  }

  private burst(duration: number, frequency: number, q: number, gain: number, pan = 0): void {
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
    this.output(envelope, pan)
    source.start(now)
    source.stop(now + duration)
  }

  setVolume(value: number): void {
    this.volume = Math.max(0, Math.min(1, value))
    if (this.master) {
      this.master.gain.value = 0.5 * this.volume
    }
  }

  shoot(smg: boolean, volume = 1, pan = 0): void {
    this.resume()
    this.burst(0.09, smg ? 2400 : 1200, 0.7, 1.0 * volume, pan)
    this.burst(0.18, smg ? 500 : 320, 0.6, 0.6 * volume, pan)
    this.tone(smg ? 160 : 90, 0.09, 0.35 * volume, 'sine', smg ? 80 : 45, pan)
  }

  hit(): void {
    this.tone(1800, 0.05, 0.3, 'triangle')
  }

  kill(): void {
    this.tone(800, 0.12, 0.24, 'sine')
    window.setTimeout(() => this.tone(400, 0.18, 0.2, 'triangle'), 70)
  }

  hurt(): void {
    this.tone(70, 0.2, 0.5, 'sine', 45)
  }

  death(): void {
    this.tone(180, 0.55, 0.4, 'sawtooth', 50)
  }

  reloadStart(): void {
    this.tone(320, 0.05, 0.28, 'square')
    window.setTimeout(() => this.tone(240, 0.05, 0.28, 'square'), 110)
  }

  reloadEnd(): void {
    this.tone(520, 0.05, 0.3, 'square')
  }

  spawn(): void {
    this.tone(300, 0.1, 0.12, 'sine', 520)
  }
}
