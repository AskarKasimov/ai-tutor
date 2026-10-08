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
            resolve(new Blob(chunks, { type: recorder.mimeType }))
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
) {
  const source = createAudioUrl(blob)
  const player = new Audio(source.url)
  const dispose = () => {
    player.onended = null
    player.onerror = null
    player.pause()
    player.removeAttribute('src')
    source.dispose()
  }
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
