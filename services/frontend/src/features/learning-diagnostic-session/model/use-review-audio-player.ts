import { useCallback, useEffect, useRef, useState } from 'react'
import { StoredAudioError } from '@/shared/api'
import { playQuestion } from '@/shared/lib'
import { useDiagnosticDependencies } from './dependencies-context'

type Notice = {
  taskId: string
  key:
    | 'learningTrainer.audioPreparing'
    | 'learningTrainer.audioMissing'
    | 'learningTrainer.speechError'
}

export function useReviewAudioPlayer() {
  const { diagnostic } = useDiagnosticDependencies()
  const [playingId, setPlayingId] = useState<string | null>(null)
  const [loadingId, setLoadingId] = useState<string | null>(null)
  const [notice, setNotice] = useState<Notice | null>(null)
  const activeController = useRef<AbortController | null>(null)
  const activeDispose = useRef<(() => void) | null>(null)
  const activeTaskId = useRef<string | null>(null)

  const stop = useCallback(() => {
    activeController.current?.abort()
    activeController.current = null
    activeDispose.current?.()
    activeDispose.current = null
    activeTaskId.current = null
    setPlayingId(null)
    setLoadingId(null)
    setNotice(null)
  }, [])

  const play = useCallback(
    async (sourceTaskId: string) => {
      if (activeTaskId.current === sourceTaskId) {
        stop()
        return
      }
      stop()
      const controller = new AbortController()
      activeController.current = controller
      activeTaskId.current = sourceTaskId
      setLoadingId(sourceTaskId)
      const isCurrent = () =>
        !controller.signal.aborted && activeController.current === controller
      const finish = () => {
        if (!isCurrent()) return
        activeController.current = null
        activeDispose.current = null
        activeTaskId.current = null
        setPlayingId(null)
        setLoadingId(null)
      }
      try {
        const audioId = sourceTaskId.startsWith('taskaudio_')
          ? sourceTaskId
          : `taskaudio_${sourceTaskId}`
        const blob = await diagnostic.fetchDiagnosticAudioFile(
          `/learn/task-audio/${audioId}/file`,
          controller.signal,
        )
        if (!isCurrent()) return
        const dispose = await playQuestion(
          blob,
          finish,
          () => {
            if (!isCurrent()) return
            finish()
            setNotice({
              taskId: sourceTaskId,
              key: 'learningTrainer.speechError',
            })
          },
          controller.signal,
        )
        if (!isCurrent()) {
          dispose()
          return
        }
        activeDispose.current = dispose
        setLoadingId(null)
        setPlayingId(sourceTaskId)
      } catch (error) {
        if (!isCurrent()) return
        finish()
        const key =
          error instanceof StoredAudioError && error.code === 'AUDIO_NOT_READY'
            ? 'learningTrainer.audioPreparing'
            : error instanceof StoredAudioError && error.status === 404
              ? 'learningTrainer.audioMissing'
              : 'learningTrainer.speechError'
        setNotice({ taskId: sourceTaskId, key })
      }
    },
    [diagnostic, stop],
  )

  useEffect(() => stop, [stop])
  return { playingId, loadingId, notice, play, stop }
}
