import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
  error?: string
}

export function useMaterialUploadQueue(userId: string, subjectId: string) {
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

  function updateItems(next: Array<{ id: string }>) {
    const positions = new Map(next.map((item, index) => [item.id, index]))
    setItems((current) =>
      [...current].sort(
        (a, b) =>
          (positions.get(a.id) ?? Infinity) - (positions.get(b.id) ?? Infinity),
      ),
    )
  }

  async function send(selected: MaterialQueueItem[]) {
    if (!selected.length || sendingRef.current) return
    sendingRef.current = true
    setSending(true)
    setError('')
    setSuccess(false)
    setSendingItemId(selected.length === 1 ? selected[0].id : null)
    let allSucceeded = true
    try {
      for (let offset = 0; offset < selected.length; offset += 48) {
        const batch = selected.slice(offset, offset + 48)
        const result = await upload.mutateAsync({
          subjectId,
          files: batch.map((item) => item.file),
        })
        // Keep ambiguous duplicate names for retry if the server reports a failure.
        const uploadedNames = new Set(
          result.uploaded.map((item) => item.filename),
        )
        const failedNames = new Map(
          result.failed.map((item) => [item.filename, item.message]),
        )
        const sentIds = new Set(batch.map((item) => item.id))
        setItems((current) =>
          current
            .filter(
              (item) =>
                !sentIds.has(item.id) ||
                !uploadedNames.has(item.file.name) ||
                failedNames.has(item.file.name),
            )
            .map((item) => ({
              ...item,
              error: sentIds.has(item.id)
                ? failedNames.get(item.file.name)
                : item.error,
            })),
        )
        if (result.failed.length > 0) {
          allSucceeded = false
          setError(
            result.failed
              .map((item) => `${item.filename}: ${item.message}`)
              .join('; '),
          )
        }
        if (result.uploaded.length !== batch.length) allSucceeded = false
      }
      setSuccess(allSucceeded)
    } catch {
      // The mutation exposes the transport error; unsent files stay queued.
    } finally {
      sendingRef.current = false
      setSending(false)
      setSendingItemId(null)
    }
  }

  useEffect(() => {
    if (!items.length && !sending) return
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [items.length, sending])

  return {
    items,
    error,
    success,
    preparing,
    isPending: sending,
    sendingItemId,
    valid: items.length > 0,
    uploadError: upload.isError
      ? upload.error instanceof Error
        ? upload.error.message
        : t('competencyMap.requestError')
      : '',
    addFile,
    removeItem,
    updateItems,
    sendAll: () => send(items),
    sendItem: (id: string) => send(items.filter((item) => item.id === id)),
  }
}
