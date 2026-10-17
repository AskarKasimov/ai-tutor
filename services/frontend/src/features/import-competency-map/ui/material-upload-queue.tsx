import { Button, Text } from '@radix-ui/themes'
import {
  ChevronDown,
  ChevronUp,
  GripVertical,
  Send,
  Trash2,
  Upload,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import styles from './material-upload-queue.module.scss'
import {
  useMaterialUploadQueue,
  type MaterialQueueItem,
} from '../model/use-material-upload-queue'
import { CourseMaterialCard } from './course-material-card'
import {
  moveMaterialQueueItem,
  moveMaterialQueueItemToIndex,
} from '../model/material-queue-order'

type MaterialUploadQueueState = ReturnType<typeof useMaterialUploadQueue>

export function MaterialReorderControls({
  name,
  disabled,
  isFirst,
  isLast,
  onMoveUp,
  onMoveDown,
}: {
  name: string
  disabled: boolean
  isFirst: boolean
  isLast: boolean
  onMoveUp: () => void
  onMoveDown: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className={styles.reorderControls}>
      <Button
        type="button"
        variant="ghost"
        size="1"
        className={styles.orderButton}
        aria-label={t('competencyMap.queueMoveUp', { name })}
        disabled={disabled || isFirst}
        onClick={onMoveUp}
      >
        <ChevronUp aria-hidden="true" size={16} />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="1"
        className={styles.dragHandle}
        aria-label={t('competencyMap.queueDragItem', { name })}
        disabled={disabled}
        draggable={!disabled}
        data-queue-drag-handle="true"
      >
        <GripVertical aria-hidden="true" size={16} />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="1"
        className={styles.orderButton}
        aria-label={t('competencyMap.queueMoveDown', { name })}
        disabled={disabled || isLast}
        onClick={onMoveDown}
      >
        <ChevronDown aria-hidden="true" size={16} />
      </Button>
    </div>
  )
}

export function MaterialUploadQueueToolbar({
  queue,
  disabled = false,
}: {
  queue: MaterialUploadQueueState
  disabled?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className={styles.queue}>
      <label
        className={styles.add}
        data-disabled={
          disabled || queue.isPending || queue.preparing || undefined
        }
      >
        <span className={styles.addIcon} aria-hidden="true">
          <Upload size={22} />
        </span>
        <span className={styles.addText}>
          <span className={styles.addTitle}>{t('competencyMap.queueAdd')}</span>
          <span className={styles.addHint}>
            {t('competencyMap.queueAddHint')}
          </span>
        </span>
        <input
          className={styles.addInput}
          type="file"
          accept=".pdf,.zip,.md,.txt,.markdown,application/pdf,application/zip,text/plain,text/markdown"
          disabled={disabled || queue.isPending || queue.preparing}
          aria-label={t('competencyMap.queueAdd')}
          onChange={(event) => {
            void queue.addFile(event.currentTarget.files?.[0])
            event.currentTarget.value = ''
          }}
        />
      </label>
      {queue.error && (
        <Text role="alert" color="red">
          {queue.error}
        </Text>
      )}
      {queue.uploadError && (
        <Text role="alert" color="red">
          {queue.uploadError}
        </Text>
      )}
      {queue.success && (
        <Text role="status" color="green">
          {t('competencyMap.materialsSuccess')}
        </Text>
      )}
    </div>
  )
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
  const queue = useMaterialUploadQueue(userId, subjectId)
  const [draggingId, setDraggingId] = useState<string | null>(null)
  const [dropTargetId, setDropTargetId] = useState<string | null>(null)
  const orderedItems = queue.items

  function moveItem(id: string, targetIndex: number) {
    const current = orderedItems.map(({ id: itemId }) => ({ id: itemId }))
    queue.updateItems(moveMaterialQueueItemToIndex(current, id, targetIndex))
  }

  return (
    <>
      <MaterialUploadQueueToolbar queue={queue} disabled={disabled} />
      {orderedItems.length > 0 && (
        <div className={styles.items} role="list">
          {orderedItems.map((item, index) => (
            <div
              className={styles.item}
              key={item.id}
              role="listitem"
              draggable={!disabled && !queue.isPending && !queue.preparing}
              data-dragging={draggingId === item.id || undefined}
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
              onDrop={(event) => {
                event.preventDefault()
                const sourceId =
                  event.dataTransfer.getData('text/plain') || draggingId
                if (sourceId) {
                  const current = orderedItems.map(({ id }) => ({ id }))
                  queue.updateItems(
                    moveMaterialQueueItem(current, sourceId, item.id),
                  )
                }
                setDraggingId(null)
                setDropTargetId(null)
              }}
              onDragLeave={() => setDropTargetId(null)}
            >
              <MaterialReorderControls
                name={item.file.name}
                disabled={disabled || queue.isPending || queue.preparing}
                isFirst={index === 0}
                isLast={index === orderedItems.length - 1}
                onMoveUp={() => moveItem(item.id, index - 1)}
                onMoveDown={() => moveItem(item.id, index + 1)}
              />
              <MaterialQueueCard
                item={item}
                disabled={disabled || queue.isPending || queue.preparing}
                dropTarget={dropTargetId === item.id}
                onRemove={() => queue.removeItem(item.id)}
                onSend={() => queue.sendItem(item.id)}
                sending={queue.sendingItemId === item.id}
              />
            </div>
          ))}
        </div>
      )}
      <MaterialUploadSubmit queue={queue} disabled={disabled} />
    </>
  )
}

export function MaterialUploadSubmit({
  queue,
  disabled = false,
}: {
  queue: MaterialUploadQueueState
  disabled?: boolean
}) {
  const { t } = useTranslation()
  if (queue.items.length === 0) return null
  return (
    <Button
      type="button"
      color="green"
      disabled={disabled || queue.isPending || !queue.valid}
      onClick={queue.sendAll}
    >
      {queue.isPending
        ? t('competencyMap.queueSending')
        : t('competencyMap.queueSendAll')}
    </Button>
  )
}

export function MaterialQueueCard({
  item,
  disabled,
  dropTarget,
  onRemove,
  onSend,
  sending,
}: {
  item: MaterialQueueItem
  disabled: boolean
  dropTarget: boolean
  onRemove: () => void
  onSend: () => void
  sending: boolean
}) {
  const { t } = useTranslation()
  const formattedSize =
    item.file.size > 1024 * 1024
      ? `${(item.file.size / (1024 * 1024)).toFixed(1)} МБ`
      : `${Math.round(item.file.size / 1024)} КБ`

  return (
    <CourseMaterialCard
      name={item.file.name}
      status={t(
        item.error
          ? 'competencyMap.queueItemError'
          : 'competencyMap.queueItemReady',
      )}
      statusColor={item.error ? 'red' : 'blue'}
      dropTarget={dropTarget}
      meta={t('competencyMap.materialsSize', { size: formattedSize })}
      actions={
        <Button
          type="button"
          size="1"
          variant="soft"
          color="red"
          disabled={disabled}
          onClick={onRemove}
          aria-label={t('competencyMap.queueRemove')}
          title={t('competencyMap.queueRemove')}
        >
          <Trash2 size={14} />
        </Button>
      }
    >
      {item.error && (
        <Text as="p" role="alert" color="red" size="1">
          {item.error}
        </Text>
      )}
      <Button
        type="button"
        size="2"
        variant="soft"
        color="green"
        disabled={disabled}
        onClick={onSend}
        aria-label={t('competencyMap.queueSendOne')}
        title={t('competencyMap.queueSendOne')}
      >
        <Send size={14} />
        {sending
          ? t('competencyMap.queueSending')
          : t('competencyMap.queueSendOne')}
      </Button>
    </CourseMaterialCard>
  )
}
