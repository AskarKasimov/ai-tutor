// Browser speech synthesis used when the stored instruction audio is not ready.

// Reads the text aloud with the browser voice. Returns a stop function, or null
// when the browser cannot speak, so callers keep their previous behavior.
export function speakInstruction(
  text: string,
  onEnd: () => void,
): (() => void) | null {
  const synth =
    typeof window === 'undefined' ? undefined : window.speechSynthesis
  if (!synth || typeof SpeechSynthesisUtterance === 'undefined' || !text.trim())
    return null
  const voices = synth.getVoices()
  const voice = voices.find((candidate) =>
    candidate.lang.toLowerCase().startsWith('ru'),
  )
  // Without a Russian voice the browser reads Russian with a foreign voice or
  // stays silent, so the stored-audio hint is the honest fallback.
  if (voices.length > 0 && !voice) return null
  synth.cancel()
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.lang = 'ru-RU'
  if (voice) utterance.voice = voice
  let done = false
  const finish = () => {
    if (done) return
    done = true
    onEnd()
  }
  utterance.onend = finish
  utterance.onerror = finish
  synth.speak(utterance)
  return () => {
    if (done) return
    done = true
    utterance.onend = null
    utterance.onerror = null
    synth.cancel()
  }
}
