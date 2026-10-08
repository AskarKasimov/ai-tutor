export function validWavBlob(): Blob {
  const bytes = new Uint8Array(46)
  const view = new DataView(bytes.buffer)
  const text = (offset: number, value: string) => {
    for (let index = 0; index < value.length; index++)
      bytes[offset + index] = value.charCodeAt(index)
  }
  text(0, 'RIFF')
  view.setUint32(4, 38, true)
  text(8, 'WAVEfmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, 16_000, true)
  view.setUint32(28, 32_000, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  text(36, 'data')
  view.setUint32(40, 2, true)
  return new Blob([bytes], { type: 'audio/wav' })
}

export function validExtensibleWavBlob(): Blob {
  const bytes = new Uint8Array(70)
  const view = new DataView(bytes.buffer)
  const text = (offset: number, value: string) => {
    for (let index = 0; index < value.length; index++)
      bytes[offset + index] = value.charCodeAt(index)
  }
  text(0, 'RIFF')
  view.setUint32(4, 62, true)
  text(8, 'WAVEfmt ')
  view.setUint32(16, 40, true)
  view.setUint16(20, 0xfffe, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, 16_000, true)
  view.setUint32(28, 32_000, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  view.setUint16(36, 22, true)
  view.setUint16(38, 16, true)
  view.setUint32(40, 4, true)
  view.setUint16(44, 1, true)
  bytes.set([0, 0, 0, 0, 16, 0, 128, 0, 0, 170, 0, 56, 155, 113], 46)
  text(60, 'data')
  view.setUint32(64, 2, true)
  return new Blob([bytes], { type: 'audio/wav' })
}
