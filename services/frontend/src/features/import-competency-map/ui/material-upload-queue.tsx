import { Button, Select, Text, TextField } from '@radix-ui/themes'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { CourseMaterialKind } from '@/entities/competency-map'
import { useUploadCourseMaterialsMutation } from '../model/use-competency-map'
import {
  MaterialArchiveError,
  MATERIAL_QUEUE_LIMIT,
  prepareMaterialFiles,
} from '../model/prepare-material-files'
import styles from './material-upload-queue.module.scss'
type QueueItem = {
  id: string
  file: File
  week: string
  kind: CourseMaterialKind
  error?: string
}
export function MaterialUploadQueue({
  userId,
  subjectId,
  disabled = false,
}: {
  userId: string
  subjectId: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const upload = useUploadCourseMaterialsMutation(userId, subjectId)
  const [items, setItems] = useState<QueueItem[]>([])
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [preparing, setPreparing] = useState(false)
  const preparingRef = useRef(false)
  const add = async (file?: File) => {
    if (!file || preparingRef.current) return
    preparingRef.current = true
    setPreparing(true)
    setError('')
    setSuccess(false)
    try {
      const prepared = await prepareMaterialFiles(file)
      const queued = items.reduce((sum, item) => sum + item.file.size, 0)
      if (
        queued + prepared.reduce((sum, item) => sum + item.size, 0) >
        MATERIAL_QUEUE_LIMIT
      )
        throw new MaterialArchiveError('queue-too-large')
      setItems((current) => [
        ...current,
        ...prepared.map((item) => ({
          id: crypto.randomUUID(),
          file: item,
          week: '1',
          kind: 'lecture' as const,
        })),
      ])
    } catch (cause) {
      const e = cause as MaterialArchiveError
      const message = t(`competencyMap.archiveErrors.${e.message}`, {
        defaultValue: t('competencyMap.queueInvalidArchive'),
      })
      setError(e.paths?.length ? `${message}: ${e.paths.join(', ')}` : message)
    } finally {
      preparingRef.current = false
      setPreparing(false)
    }
  }
  const valid =
    items.length > 0 &&
    items.every((item) => /^\d+$/.test(item.week) && Number(item.week) > 0)
  return (
    <div className={styles.queue}>
      <label className={styles.add}>
        <span>{t('competencyMap.queueAdd')}</span>
        <input
          type="file"
          accept=".pdf,.zip,application/pdf,application/zip"
          disabled={disabled || upload.isPending || preparing}
          onChange={(event) => {
            void add(event.currentTarget.files?.[0])
            event.currentTarget.value = ''
          }}
        />
      </label>
      {error && (
        <Text role="alert" color="red">
          {error}
        </Text>
      )}
      {success && (
        <Text role="status" color="green">
          {t('competencyMap.materialsSuccess')}
        </Text>
      )}
      {items.map((item) => (
        <div className={styles.row} key={item.id}>
          <div>
            <Text className={styles.name}>{item.file.name}</Text>
            {item.error && (
              <Text as="p" role="alert" color="red" size="1">
                {item.error}
              </Text>
            )}
          </div>
          <label>
            {t('courseGraph.weekLabel')}
            <TextField.Root
              type="number"
              min="1"
              value={item.week}
              disabled={disabled || upload.isPending || preparing}
              onChange={(event) =>
                setItems((current) =>
                  current.map((row) =>
                    row.id === item.id
                      ? { ...row, week: event.target.value }
                      : row,
                  ),
                )
              }
            />
          </label>
          <label>
            {t('courseGraph.kindLabel')}
            <Select.Root
              value={item.kind}
              disabled={disabled || upload.isPending || preparing}
              onValueChange={(kind) =>
                setItems((current) =>
                  current.map((row) =>
                    row.id === item.id
                      ? { ...row, kind: kind as CourseMaterialKind }
                      : row,
                  ),
                )
              }
            >
              <Select.Trigger />
              <Select.Content>
                <Select.Item value="lecture">
                  {t('courseGraph.kinds.lecture')}
                </Select.Item>
                <Select.Item value="lab">
                  {t('courseGraph.kinds.lab')}
                </Select.Item>
                <Select.Item value="tutorial">
                  {t('courseGraph.kinds.tutorial')}
                </Select.Item>
              </Select.Content>
            </Select.Root>
          </label>
          <Button
            type="button"
            variant="soft"
            color="red"
            disabled={disabled || upload.isPending || preparing}
            onClick={() =>
              setItems((current) => current.filter((row) => row.id !== item.id))
            }
          >
            {t('competencyMap.queueRemove')}
          </Button>
        </div>
      ))}
      {upload.isError && (
        <Text role="alert" color="red">
          {upload.error instanceof Error
            ? upload.error.message
            : t('competencyMap.requestError')}
        </Text>
      )}
      {items.length > 0 && (
        <Button
          type="button"
          color="green"
          disabled={disabled || upload.isPending || !valid}
          onClick={() =>
            upload.mutate(
              {
                subjectId,
                items: items.map((item) => ({
                  clientId: item.id,
                  file: item.file,
                  week: Number(item.week),
                  kind: item.kind,
                })),
              },
              {
                onSuccess: (result) => {
                  const persisted = new Set(
                    result.results
                      .filter((entry) => entry.material)
                      .map((entry) => entry.clientId),
                  )
                  setItems((current) =>
                    current
                      .filter((item) => !persisted.has(item.id))
                      .map((item) => ({
                        ...item,
                        error: result.results.find(
                          (entry) => entry.clientId === item.id,
                        )?.error?.message,
                      })),
                  )
                  setSuccess(
                    persisted.size > 0 &&
                      persisted.size === result.results.length,
                  )
                },
              },
            )
          }
        >
          {upload.isPending
            ? t('competencyMap.uploading')
            : t('competencyMap.queueSendAll')}
        </Button>
      )}
    </div>
  )
}
