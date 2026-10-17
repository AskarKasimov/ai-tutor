import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type {
  CourseMaterialKind,
  CourseMaterialKindOption,
  CourseMaterialUploadResult,
} from '@/entities/competency-map'
import { useUploadCourseMaterialsMutation } from './use-competency-map'
import {
  MaterialArchiveError,
  MATERIAL_QUEUE_LIMIT,
  assertMaterialQueueCapacity,
  prepareMaterialFiles,
} from './prepare-material-files'

export type MaterialQueueItem = {
  id: string
  file: File
  week: string
  kind: CourseMaterialKind
  error?: string
}

export function useMaterialUploadQueue(
  userId: string,
  subjectId: string,
  kinds: CourseMaterialKindOption[],
) {
  const { t } = useTranslation()
  const upload = useUploadCourseMaterialsMutation(userId, subjectId)
  const [items, setItems] = useState<MaterialQueueItem[]>([])
  const [notification, setNotification] = useState<{
    type: 'success' | 'error' | 'warning'
    message: string
  } | null>(null)
  const [sending, setSending] = useState(false)
  const sendingRef = useRef(false)
  const sendRunRef = useRef<{ cancelled: boolean } | null>(null)
  const [preparing, setPreparing] = useState(false)
  const [sendingItemId, setSendingItemId] = useState<string | null>(null)
  const preparingRef = useRef(false)
  const subjectRef = useRef(subjectId)

  useEffect(
    () => () => {
      if (sendRunRef.current) {
        sendRunRef.current.cancelled = true
        sendRunRef.current = null
      }
    },
    [],
  )

  useEffect(() => {
    if (subjectRef.current === subjectId) return
    subjectRef.current = subjectId
    setItems([])
    setNotification(null)
  }, [subjectId])

  async function addFile(file?: File) {
    if (!file || preparingRef.current) return
    preparingRef.current = true
    setPreparing(true)
    setNotification(null)
    try {
      const prepared = await prepareMaterialFiles(file)
      assertMaterialQueueCapacity(items.length, prepared.length)
      const queued = items.reduce((sum, item) => sum + item.file.size, 0)
      if (
        queued + prepared.reduce((sum, item) => sum + item.size, 0) >
        MATERIAL_QUEUE_LIMIT
      )
        throw new MaterialArchiveError('queue-too-large')
      setItems((current) => [
        ...current,
        ...prepared.map((preparedFile) => ({
          id: crypto.randomUUID(),
          file: preparedFile,
          week: '1',
          kind: kinds[0]?.code ?? '',
        })),
      ])
    } catch (cause) {
      const archiveError = cause as MaterialArchiveError
      const message = t(`competencyMap.archiveErrors.${archiveError.message}`, {
        defaultValue: t('competencyMap.queueInvalidArchive'),
      })
      setNotification({
        type: 'error',
        message: archiveError.paths?.length
          ? `${message}: ${archiveError.paths.join(', ')}`
          : message,
      })
    } finally {
      preparingRef.current = false
      setPreparing(false)
    }
  }

  function removeItem(id: string) {
    setItems((current) => current.filter((item) => item.id !== id))
  }

  function updateItems(next: Array<{ id: string; week: string }>) {
    const positions = new Map(next.map((item, index) => [item.id, index]))
    const weeks = new Map(next.map((item) => [item.id, item.week]))
    setItems((current) =>
      [...current]
        .sort(
          (a, b) =>
            (positions.get(a.id) ?? Number.MAX_SAFE_INTEGER) -
            (positions.get(b.id) ?? Number.MAX_SAFE_INTEGER),
        )
        .map((item) => ({ ...item, week: weeks.get(item.id) ?? item.week })),
    )
  }

  function updateWeek(id: string, week: string) {
    setItems((current) =>
      current.map((item) => (item.id === id ? { ...item, week } : item)),
    )
  }

  function updateKind(id: string, kind: CourseMaterialKind) {
    setItems((current) =>
      current.map((item) => (item.id === id ? { ...item, kind } : item)),
    )
  }

  async function send(selected: MaterialQueueItem[]) {
    if (
      !selected.length ||
      sendingRef.current ||
      !selected.every(
        (item) =>
          /^\d+$/.test(item.week) &&
          Number(item.week) > 0 &&
          kinds.some((kind) => kind.code === item.kind),
      )
    )
      return
    sendingRef.current = true
    const run = { cancelled: false }
    sendRunRef.current = run
    setSending(true)
    setNotification(null)
    setSendingItemId(selected.length === 1 ? selected[0].id : null)
    let complete = true
    try {
      for (let offset = 0; offset < selected.length; offset += 48) {
        if (run.cancelled) return
        const batch = selected.slice(offset, offset + 48)
        const result: CourseMaterialUploadResult = await upload.mutateAsync({
          subjectId,
          items: batch.map((item) => ({
            clientId: item.id,
            file: item.file,
            week: Number(item.week),
            kind: item.kind,
          })),
        })
        if (run.cancelled) return
        const persisted = new Set(
          result.results
            .filter((entry) => entry.material)
            .map((entry) => entry.clientId),
        )
        const responses = new Map(
          result.results.map((entry) => [entry.clientId, entry]),
        )
        setItems((current) =>
          current
            .filter((item) => !persisted.has(item.id))
            .map((item) => ({
              ...item,
              error: responses.get(item.id)?.error?.message ?? item.error,
            })),
        )
        if (persisted.size !== batch.length) complete = false
      }
      setNotification({
        type: complete ? 'success' : 'warning',
        message: t(
          complete
            ? 'competencyMap.materialsSuccess'
            : 'competencyMap.uploadResultPartial',
        ),
      })
    } catch (cause) {
      if (run.cancelled) return
      setNotification({
        type: 'error',
        message:
          cause instanceof Error
            ? cause.message
            : t('competencyMap.requestError'),
      })
    } finally {
      if (sendRunRef.current === run) {
        sendRunRef.current = null
        sendingRef.current = false
        setSending(false)
        setSendingItemId(null)
      }
    }
  }
  function sendAll() {
    return send(items)
  }
  function sendItem(id: string) {
    return send(items.filter((item) => item.id === id))
  }

  const valid =
    items.length > 0 &&
    items.every(
      (item) =>
        /^\d+$/.test(item.week) &&
        Number(item.week) > 0 &&
        kinds.some((kind) => kind.code === item.kind),
    )

  return {
    items,
    setItems,
    notification,
    dismissNotification: () => setNotification(null),
    preparing,
    isPending: sending,
    sendingItemId,
    valid,
    addFile,
    removeItem,
    updateItems,
    updateWeek,
    updateKind,
    sendAll,
    sendItem,
  }
}
