// The spoken phrase "Я пропускаю вопрос." submitted as an answer when the
// student skips a question; the regular voice pipeline grades it.
import { fetchStaticAsset } from '@/shared/api'

let cached: Promise<Blob> | null = null

export function loadSkipAnswer(): Promise<Blob> {
  cached ??= fetchStaticAsset('audio/skip-answer.wav')
    .then(async (response) => {
      if (!response.ok) throw new Error('Skip answer audio is unavailable')
      return new Blob([await response.arrayBuffer()], { type: 'audio/wav' })
    })
    .catch((error: unknown) => {
      cached = null
      throw error
    })
  return cached
}
