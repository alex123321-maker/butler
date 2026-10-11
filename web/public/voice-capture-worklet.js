/* A fixed mono PCM capture worklet. Network and recognition are absent. */
class ButlerVoiceCapture extends AudioWorkletProcessor {
  process(inputs) {
    const channel = inputs[0]?.[0]
    if (channel?.length && channel.length <= 128) this.port.postMessage(channel.slice())
    return true
  }
}
registerProcessor('butler-voice-capture', ButlerVoiceCapture)
