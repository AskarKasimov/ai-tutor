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
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [sending, setSending] = useState(false)
  const sendingRef = useRef(false)
  const [preparing, setPreparing] = useState(false)
  const [sendingItemId, setSendingItemId] = useState<string | null>(null)
  const preparingRef = useRef(false)
  const subjectRef = useRef(subjectId)

  useEffect(() => {
    if (subjectRef.current === subjectId) return
    subjectRef.current = subjectId
    setItems([])
    setError('')
    setSuccess(false)
  }, [subjectId])

  async function addFile(file?: File) {
    if (!file || preparingRef.current) return
    preparingRef.current = true
    setPreparing(true)
    setError('')
    setSuccess(false)
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
      setError(
        archiveError.paths?.length
          ? `${message}: ${archiveError.paths.join(', ')}`
          : message,
      )
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
    setSending(true)
    setSuccess(false)
    setSendingItemId(selected.length === 1 ? selected[0].id : null)
    let complete = true
    try {
      for (let offset = 0; offset < selected.length; offset += 48) {
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
      setSuccess(complete)
    } catch {
      // The mutation provides the error; keep failed and unsent files queued.
    } finally {
      sendingRef.current = false
      setSending(false)
      setSendingItemId(null)
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
    error,
    success,
    preparing,
    isPending: sending,
    sendingItemId,
    valid,
    uploadError: upload.isError
      ? upload.error instanceof Error
        ? upload.error.message
        : t('competencyMap.requestError')
      : '',
    addFile,
    removeItem,
    updateItems,
    updateWeek,
    updateKind,
    sendAll,
    sendItem,
  }
}
