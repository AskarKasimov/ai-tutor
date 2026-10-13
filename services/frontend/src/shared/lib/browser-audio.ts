// Browser adapters for the browser recording, playback, and audio URL adapters.
export type Recording = {
  stream: MediaStream
  stop: () => Promise<Blob>
  dispose: () => void
}

export class AudioPlaybackError extends Error {
  constructor(readonly kind: 'decode' | 'source') {
    super(`Audio ${kind} failed`)
    this.name = 'AudioPlaybackError'
  }
}

export async function startRecording(): Promise<Recording> {
  if (
    !navigator.mediaDevices?.getUserMedia ||
    typeof MediaRecorder === 'undefined'
  ) {
    throw new Error('unavailable')
  }
  // Match the STT contract; browser defaults may use an unsupported codec.
  const mimeType = ['audio/webm;codecs=opus', 'audio/ogg;codecs=opus'].find(
    (type) => MediaRecorder.isTypeSupported(type),
  )
  if (!mimeType) throw new Error('unavailable')
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
  try {
    const recorder = new MediaRecorder(stream, { mimeType })
    const chunks: Blob[] = []
    recorder.ondataavailable = (event) => {
      if (event.data.size) chunks.push(event.data)
    }
    recorder.start()
    const release = () => stream.getTracks().forEach((track) => track.stop())
    return {
      stream,
      stop: () =>
        new Promise((resolve, reject) => {
          recorder.onstop = () => {
            release()
            resolve(
              prepareSpeech(new Blob(chunks, { type: recorder.mimeType })),
            )
          }
          recorder.onerror = () => {
            release()
            reject(new Error('recording'))
          }
          recorder.stop()
        }),
      dispose: () => {
        recorder.onstop = release
        if (recorder.state !== 'inactive') recorder.stop()
        release()
      },
    }
  } catch (error) {
    stream.getTracks().forEach((track) => track.stop())
    throw error
  }
}

export async function playQuestion(
  blob: Blob,
  onEnd: () => void,
  onError: (error: AudioPlaybackError) => void,
  signal?: AbortSignal,
) {
  signal?.throwIfAborted()
  const source = createAudioUrl(blob)
  const player = new Audio(source.url)
  let disposed = false
  const dispose = () => {
    if (disposed) return
    disposed = true
    signal?.removeEventListener('abort', dispose)
    player.onended = null
    player.onerror = null
    player.pause()
    player.removeAttribute('src')
    source.dispose()
  }
  signal?.addEventListener('abort', dispose, { once: true })
  player.onended = () => {
    dispose()
    onEnd()
  }
  player.onerror = () => {
    const code = player.error?.code
    dispose()
    onError(
      new AudioPlaybackError(
        code === MediaError.MEDIA_ERR_DECODE ? 'decode' : 'source',
      ),
    )
  }
  try {
    await player.play()
    signal?.throwIfAborted()
    return dispose
  } catch (error) {
    dispose()
    if (error instanceof DOMException && error.name === 'NotSupportedError')
      throw new AudioPlaybackError('source')
    throw error
  }
}

export function createAudioUrl(blob: Blob) {
  const url = URL.createObjectURL(blob)
  return { url, dispose: () => URL.revokeObjectURL(url) }
}

// Spoken answers stop recording automatically after a minute and a half.
export const MAX_ANSWER_SECONDS = 90

// Speech recognition accepts short compressed recordings as they are. A longer
// one becomes 16 kHz mono WAV, which the backend cuts at pauses into parts the
// recognizer accepts. When the browser cannot decode it, the recording is sent
// unchanged.
export async function prepareSpeech(blob: Blob, shortSeconds = 20) {
  if (typeof OfflineAudioContext === 'undefined' || !blob.size) return blob
  try {
    const rate = 16_000
    const decoded = await new OfflineAudioContext(
      1,
      rate,
      rate,
    ).decodeAudioData(await blob.arrayBuffer())
    if (decoded.duration <= shortSeconds) return blob
    const length = Math.ceil(decoded.duration * rate)
    const context = new OfflineAudioContext(1, length, rate)
    const source = context.createBufferSource()
    source.buffer = decoded
    source.connect(context.destination)
    source.start()
    const samples = (await context.startRendering()).getChannelData(0)
    const wav = new DataView(new ArrayBuffer(44 + samples.length * 2))
    const text = (offset: number, value: string) =>
      [...value].forEach((char, i) =>
        wav.setUint8(offset + i, char.charCodeAt(0)),
      )
    text(0, 'RIFF')
    wav.setUint32(4, 36 + samples.length * 2, true)
    text(8, 'WAVEfmt ')
    wav.setUint32(16, 16, true)
    wav.setUint16(20, 1, true)
    wav.setUint16(22, 1, true)
    wav.setUint32(24, rate, true)
    wav.setUint32(28, rate * 2, true)
    wav.setUint16(32, 2, true)
    wav.setUint16(34, 16, true)
    text(36, 'data')
    wav.setUint32(40, samples.length * 2, true)
    samples.forEach((sample, i) => {
      const clamped = Math.max(-1, Math.min(1, sample))
      wav.setInt16(44 + i * 2, clamped * 0x7fff, true)
    })
    return new Blob([wav], { type: 'audio/wav' })
  } catch {
    return blob
  }
}
