export const encodeVoiceWav = (samples: Float32Array): ArrayBuffer => {
  if (samples.length < 1 || samples.length > 120 * 16000) throw new Error('recording_bound')
  const wav = new ArrayBuffer(44 + samples.length * 2)
  const view = new DataView(wav)
  const text = (at: number, value: string) => { for (let i = 0; i < value.length; i++) view.setUint8(at + i, value.charCodeAt(i)) }
  text(0, 'RIFF'); view.setUint32(4, wav.byteLength - 8, true); text(8, 'WAVE'); text(12, 'fmt ')
  view.setUint32(16, 16, true); view.setUint16(20, 1, true); view.setUint16(22, 1, true)
  view.setUint32(24, 16000, true); view.setUint32(28, 32000, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true)
  text(36, 'data'); view.setUint32(40, samples.length * 2, true)
  samples.forEach((sample, i) => view.setInt16(44 + i * 2, Math.round(Math.max(-1, Math.min(1, sample)) * (sample < 0 ? 32768 : 32767)), true))
  return wav
}

// Capture exists only between explicit UI actions. No permission request at
// page load, no speech recognition in the browser, no network or text authority.
export class VoiceCapture {
  private stream?: MediaStream
  private context?: AudioContext
  private node?: AudioWorkletNode
  private chunks: Float32Array[] = []
  private count = 0
  private timer?: ReturnType<typeof setTimeout>
  async start(onLimit: () => void): Promise<void> {
    if (this.stream || this.context) throw new Error('already_recording')
    try {
      this.stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true }, video: false })
      this.context = new AudioContext({ sampleRate: 16000 })
      if (this.context.sampleRate !== 16000) throw new Error('sample_rate_unavailable')
      await this.context.audioWorklet.addModule('/voice-capture-worklet.js')
      this.chunks = []; this.count = 0
      this.node = new AudioWorkletNode(this.context, 'butler-voice-capture', { channelCount: 1, channelCountMode: 'explicit' })
      this.node.port.onmessage = (event: MessageEvent<Float32Array>) => {
        if (!(event.data instanceof Float32Array) || event.data.length > 128) return
        const remaining = 120 * 16000 - this.count
        if (remaining <= 0) { onLimit(); return }
        const copy = event.data.slice(0, remaining); this.chunks.push(copy); this.count += copy.length
      }
      const silence = this.context.createGain(); silence.gain.value = 0
      this.context.createMediaStreamSource(this.stream).connect(this.node)
      this.node.connect(silence); silence.connect(this.context.destination)
      await this.context.resume()
      this.timer = setTimeout(onLimit, 120000)
    } catch (error) { await this.cancel(); throw error }
  }
  async stop(): Promise<ArrayBuffer> {
    await this.release()
    const samples = new Float32Array(this.count); let at = 0
    for (const chunk of this.chunks) { samples.set(chunk, at); at += chunk.length }
    this.chunks = []; this.count = 0
    return encodeVoiceWav(samples)
  }
  async cancel(): Promise<void> { await this.release(); this.chunks = []; this.count = 0 }
  private async release(): Promise<void> {
    clearTimeout(this.timer); this.timer = undefined
    this.node?.disconnect(); if (this.node) this.node.port.onmessage = null; this.node = undefined
    this.stream?.getTracks().forEach(track => track.stop()); this.stream = undefined
    const context = this.context; this.context = undefined
    if (context && context.state !== 'closed') await context.close()
  }
}
