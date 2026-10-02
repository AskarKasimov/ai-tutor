const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')

export class VoiceApiError extends Error {
  readonly status: number

  constructor(operation: string, status: number) {
    super(`${operation} failed: ${status}`)
    this.name = 'VoiceApiError'
    this.status = status
  }
}

function requestSignal(signal: AbortSignal) {
  return AbortSignal.any([signal, AbortSignal.timeout(120_000)])
}

export async function transcribeRecording(blob: Blob, signal: AbortSignal): Promise<{ id: string; text: string }> {
  const extension = blob.type.includes('ogg') ? 'ogg' : blob.type.includes('wav') ? 'wav' : blob.type.includes('mp4') ? 'm4a' : 'webm'
  const form = new FormData()
  form.append('audio', blob, `answer.${extension}`)
  const response = await fetch(`${apiBase}/voice/transcriptions`, { method: 'POST', credentials: 'include', body: form, signal: requestSignal(signal) })
  if (!response.ok) throw new VoiceApiError('Transcription', response.status)
  const data: unknown = await response.json()
  if (!data || typeof data !== 'object' || !('id' in data) || typeof data.id !== 'string' || !data.id || !('text' in data) || typeof data.text !== 'string' || !data.text.trim()) {
    throw new Error('Transcription is empty')
  }
  return { id: data.id, text: data.text.trim() }
}

export async function synthesizeQuestion(text: string, signal: AbortSignal): Promise<Blob> {
  const response = await fetch(`${apiBase}/voice/syntheses`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
    signal: requestSignal(signal),
  })
  if (!response.ok) throw new VoiceApiError('Synthesis', response.status)
  const audio = await response.blob()
  if (!audio.size) throw new Error('Synthesis audio is empty')
  return audio
}
