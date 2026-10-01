// Browser adapters for the disposable voice trainer prototype.
export type Recording = { stop: () => Promise<Blob>; dispose: () => void }

export async function startRecording(): Promise<Recording> {
  if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
    throw new Error('unavailable')
  }
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
  try {
    const recorder = new MediaRecorder(stream)
    const chunks: Blob[] = []
    recorder.ondataavailable = (event) => { if (event.data.size) chunks.push(event.data) }
    recorder.start()
    const release = () => stream.getTracks().forEach((track) => track.stop())
    return {
      stop: () => new Promise((resolve, reject) => {
        recorder.onstop = () => { release(); resolve(new Blob(chunks, { type: recorder.mimeType })) }
        recorder.onerror = () => { release(); reject(new Error('recording')) }
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

export async function playQuestion(blob: Blob, onEnd: () => void, onError: () => void) {
  const source = createAudioUrl(blob)
  const player = new Audio(source.url)
  const dispose = () => {
    player.onended = null
    player.onerror = null
    player.pause()
    player.removeAttribute('src')
    source.dispose()
  }
  player.onended = () => { dispose(); onEnd() }
  player.onerror = () => { dispose(); onError() }
  try {
    await player.play()
    return dispose
  } catch (error) {
    dispose()
    throw error
  }
}

export function createAudioUrl(blob: Blob) {
  const url = URL.createObjectURL(blob)
  return { url, dispose: () => URL.revokeObjectURL(url) }
}
