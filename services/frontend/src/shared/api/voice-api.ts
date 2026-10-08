export class VoiceApiError extends Error {
  constructor(
    operation: string,
    readonly status: number,
  ) {
    super(`${operation} failed: ${status}`)
    this.name = 'VoiceApiError'
  }
}
import { apiFetch } from './api-fetch'
const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)

function requestSignal(signal: AbortSignal) {
  return AbortSignal.any([signal, AbortSignal.timeout(120_000)])
}

export async function transcribeRecording(
  blob: Blob,
  signal: AbortSignal,
): Promise<{ id: string; text: string }> {
  const extension = blob.type.includes('ogg')
    ? 'ogg'
    : blob.type.includes('wav')
      ? 'wav'
      : blob.type.includes('mp4')
        ? 'm4a'
        : 'webm'
  const form = new FormData()
  form.append('audio', blob, `answer.${extension}`)
  const response = await apiFetch(`${apiBase}/voice/transcriptions`, {
    method: 'POST',
    credentials: 'include',
    body: form,
    signal: requestSignal(signal),
  })
  if (!response.ok) throw new VoiceApiError('Transcription', response.status)
  const data: unknown = await response.json()
  if (
    !data ||
    typeof data !== 'object' ||
    !('id' in data) ||
    typeof data.id !== 'string' ||
    !data.id ||
    !('text' in data) ||
    typeof data.text !== 'string' ||
    !data.text.trim()
  ) {
    throw new Error('Transcription is empty')
  }
  return { id: data.id, text: data.text.trim() }
}

export async function fetchStoredAudio(
  audioUrl: string,
  signal: AbortSignal,
): Promise<Blob> {
  if (!/^\/task-audio\/[A-Za-z0-9_-]{1,256}\/file$/.test(audioUrl))
    throw new Error('Stored audio URL is invalid')
  const response = await apiFetch(`${apiBase}${audioUrl}`, {
    credentials: 'include',
    signal: requestSignal(signal),
  })
  if (!response.ok) throw new VoiceApiError('Stored audio', response.status)
  if (
    response.headers
      .get('Content-Type')
      ?.split(';', 1)[0]
      .trim()
      .toLowerCase() !== 'audio/wav'
  )
    throw new Error('Stored audio has an invalid content type')
  const audio = await response.blob()
  if (!audio.size) throw new Error('Stored audio is empty')
  signal.throwIfAborted()
  return audio
}
