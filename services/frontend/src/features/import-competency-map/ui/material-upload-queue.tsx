import { Button, Select, Text, TextField } from '@radix-ui/themes'
import { ChevronDown, ChevronUp, GripVertical } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type {
  CourseMaterialKind,
  CourseMaterialKindOption,
} from '@/entities/competency-map'
import { useUploadCourseMaterialsMutation } from '../model/use-competency-map'
import {
  MaterialArchiveError,
  MATERIAL_QUEUE_LIMIT,
  prepareMaterialFiles,
} from '../model/prepare-material-files'
import {
  moveMaterialQueueItem,
  moveMaterialQueueItemToIndex,
  sortMaterialsByWeek,
} from '../model/material-queue-order'
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
  kinds,
  disabled = false,
}: {
  userId: string
  subjectId: string
  kinds: CourseMaterialKindOption[]
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const upload = useUploadCourseMaterialsMutation(userId, subjectId)
  const [items, setItems] = useState<QueueItem[]>([])
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [preparing, setPreparing] = useState(false)
  const [draggingId, setDraggingId] = useState<string | null>(null)
  const [dropTargetId, setDropTargetId] = useState<string | null>(null)
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
          kind: kinds[0]?.code ?? '',
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
    items.every(
      (item) =>
        /^\d+$/.test(item.week) &&
        Number(item.week) > 0 &&
        kinds.some((kind) => kind.code === item.kind),
    )
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
      <div className={styles.items} role="list">
        {items.map((item) => (
          <div
            className={`${styles.item}${draggingId === item.id ? ` ${styles.dragging}` : ''}`}
            key={item.id}
            role="listitem"
            draggable={!disabled && !upload.isPending && !preparing}
            onDragStart={(event) => {
              const target = event.target
              if (
                target instanceof HTMLElement &&
                target.closest('button, input, [role="combobox"]') &&
                !target.closest('[data-queue-drag-handle]')
              ) {
                event.preventDefault()
                return
              }
              setDraggingId(item.id)
              event.dataTransfer.effectAllowed = 'move'
              event.dataTransfer.setData('text/plain', item.id)
            }}
            onDragEnd={() => {
              setDraggingId(null)
              setDropTargetId(null)
            }}
            onDragOver={(event) => {
              event.preventDefault()
              setDropTargetId(item.id)
              event.dataTransfer.dropEffect = 'move'
            }}
            onDragLeave={() => setDropTargetId(null)}
            onDrop={(event) => {
              event.preventDefault()
              const sourceId =
                event.dataTransfer.getData('text/plain') || draggingId
              if (sourceId)
                setItems((current) =>
                  moveMaterialQueueItem(current, sourceId, item.id),
                )
              setDraggingId(null)
              setDropTargetId(null)
            }}
          >
            <div className={styles.reorderControls}>
              <Button
                type="button"
                variant="ghost"
                className={styles.dragHandle}
                aria-label={t('competencyMap.queueDragItem', {
                  name: item.file.name,
                })}
                disabled={disabled || upload.isPending || preparing}
                draggable={!disabled && !upload.isPending && !preparing}
                data-queue-drag-handle="true"
              >
                <GripVertical aria-hidden="true" size={20} />
              </Button>
              <div className={styles.orderButtons}>
                <Button
                  type="button"
                  variant="ghost"
                  size="2"
                  className={styles.orderButton}
                  aria-label={t('competencyMap.queueMoveUp', {
                    name: item.file.name,
                  })}
                  disabled={
                    disabled ||
                    upload.isPending ||
                    preparing ||
                    items[0]?.id === item.id
                  }
                  onClick={() =>
                    setItems((current) => {
                      const index = current.findIndex(
                        (row) => row.id === item.id,
                      )
                      return moveMaterialQueueItemToIndex(
                        current,
                        item.id,
                        index - 1,
                      )
                    })
                  }
                >
                  <ChevronUp aria-hidden="true" size={20} />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="2"
                  className={styles.orderButton}
                  aria-label={t('competencyMap.queueMoveDown', {
                    name: item.file.name,
                  })}
                  disabled={
                    disabled ||
                    upload.isPending ||
                    preparing ||
                    items.at(-1)?.id === item.id
                  }
                  onClick={() =>
                    setItems((current) => {
                      const index = current.findIndex(
                        (row) => row.id === item.id,
                      )
                      return moveMaterialQueueItemToIndex(
                        current,
                        item.id,
                        index + 1,
                      )
                    })
                  }
                >
                  <ChevronDown aria-hidden="true" size={20} />
                </Button>
              </div>
            </div>
            <div
              className={`${styles.row}${dropTargetId === item.id ? ` ${styles.dropTarget}` : ''}`}
            >
              <div>
                <Text className={styles.name}>{item.file.name}</Text>
                {item.error && (
                  <Text as="p" role="alert" color="red" size="1">
                    {item.error}
                  </Text>
                )}
              </div>
              <label className={styles.weekField}>
                <span>{t('courseGraph.weekLabel')}</span>
                <TextField.Root
                  className={styles.weekInput}
                  type="number"
                  min="1"
                  value={item.week}
                  disabled={disabled || upload.isPending || preparing}
                  onChange={(event) =>
                    setItems((current) =>
                      sortMaterialsByWeek(
                        current.map((row) =>
                          row.id === item.id
                            ? { ...row, week: event.target.value }
                            : row,
                        ),
                      ),
                    )
                  }
                />
              </label>
              <div className={styles.kindField}>
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
                  <Select.Trigger aria-label={t('courseGraph.kindLabel')} />
                  <Select.Content>
                    {kinds.map((kind) => (
                      <Select.Item key={kind.code} value={kind.code}>
                        {kind.label}
                      </Select.Item>
                    ))}
                  </Select.Content>
                </Select.Root>
              </div>
              <Button
                type="button"
                variant="soft"
                color="red"
                disabled={disabled || upload.isPending || preparing}
                onClick={() =>
                  setItems((current) =>
                    current.filter((row) => row.id !== item.id),
                  )
                }
              >
                {t('competencyMap.queueRemove')}
              </Button>
            </div>
          </div>
        ))}
      </div>
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
