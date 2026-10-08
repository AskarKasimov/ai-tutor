// An offline WAV cue for demo synthesis; it does not pretend to read the question.
export function createDemoAudio(): ArrayBuffer {
  const rate = 8000
  const samples = 3200
  const data = new ArrayBuffer(44 + samples * 2)
  const view = new DataView(data)
  function text(offset: number, value: string) {
    for (let i = 0; i < value.length; i++)
      view.setUint8(offset + i, value.charCodeAt(i))
  }
  text(0, 'RIFF')
  view.setUint32(4, data.byteLength - 8, true)
  text(8, 'WAVE')
  text(12, 'fmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, rate, true)
  view.setUint32(28, rate * 2, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  text(36, 'data')
  view.setUint32(40, samples * 2, true)
  for (let i = 0; i < samples; i++) {
    const envelope = Math.min(i / 160, (samples - i) / 160, 1)
    view.setInt16(
      44 + i * 2,
      Math.round(Math.sin((i / rate) * Math.PI * 2 * 440) * envelope * 2000),
      true,
    )
  }
  return data
}
