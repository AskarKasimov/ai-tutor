import { useEffect, useRef, useState } from 'react'
import { useLearningPlanDependencies } from './dependencies-context'

// Recognition is separate from grading; a late preview cannot replace a new task.
export function useAnswerPreview(identity: string) {
  const { learningPlan } = useLearningPlanDependencies()
  const [data, setData] = useState<{ id: string; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>()
  const request = useRef<AbortController | null>(null)
  const generation = useRef(0)
  const lastAudio = useRef<Blob | null>(null)
  const clear = () => {
    generation.current++
    request.current?.abort()
    request.current = null
    lastAudio.current = null
    setData(null)
    setError(undefined)
    setBusy(false)
  }
  useEffect(() => {
    clear()
    return () => {
      generation.current++
      request.current?.abort()
      request.current = null
    }
  }, [identity])
  const recognize = async (audio: Blob) => {
    if (request.current) return
    const controller = new AbortController()
    request.current = controller
    lastAudio.current = audio
    const current = generation.current
    setData(null)
    setError(undefined)
    setBusy(true)
    try {
      const result = await learningPlan.previewAnswer(audio, controller.signal)
      if (current === generation.current && !controller.signal.aborted)
        setData(result)
    } catch (cause) {
      if (current === generation.current && !controller.signal.aborted)
        setError(cause)
    } finally {
      if (request.current === controller) request.current = null
      if (current === generation.current) setBusy(false)
    }
  }
  return {
    data,
    busy,
    error,
    clear,
    recognize,
    retry: () => lastAudio.current && recognize(lastAudio.current),
  }
}
