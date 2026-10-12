import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { sessionStyles } from './trainer-shared'
import styles from './training-workspace.module.scss'

const minimumWidth = 320
const defaultWidth = 420

export function TrainingWorkspace({
  children,
  chat,
}: {
  children: ReactNode
  chat: ReactNode
}) {
  const { t } = useTranslation()
  const workspace = useRef<HTMLElement>(null)
  const drag = useRef<{ pointerId: number; x: number; width: number } | null>(
    null,
  )
  const [width, setWidth] = useState(defaultWidth)
  const [availableWidth, setAvailableWidth] = useState(1200)
  const [dragging, setDragging] = useState(false)
  const maximumWidth = Math.max(
    minimumWidth,
    Math.min(720, availableWidth * 0.65, availableWidth - minimumWidth),
  )
  const panelWidth = Math.round(
    Math.max(minimumWidth, Math.min(maximumWidth, width)),
  )

  useEffect(() => {
    const node = workspace.current
    if (!node) return
    const observer = new ResizeObserver(() => {
      const measured = node.getBoundingClientRect().width - 12
      if (measured > 0) setAvailableWidth(measured)
    })
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  return (
    <main
      ref={workspace}
      className={`${sessionStyles.workspace} ${styles.workspace}`}
      data-variant="training"
      data-split
      data-dragging={dragging || undefined}
      style={{ '--training-chat-width': `${panelWidth}px` } as CSSProperties}
    >
      <div className={styles.question}>{children}</div>
      <div
        className={styles.handle}
        role="separator"
        tabIndex={0}
        aria-label={t('training.chatResize')}
        aria-orientation="vertical"
        aria-controls="training-chat-panel"
        aria-valuemin={minimumWidth}
        aria-valuemax={Math.floor(maximumWidth)}
        aria-valuenow={panelWidth}
        aria-valuetext={t('training.chatWidth', { width: panelWidth })}
        onPointerDown={(event) => {
          if (event.button !== 0) return
          event.preventDefault()
          event.currentTarget.setPointerCapture(event.pointerId)
          const measured = workspace.current!.getBoundingClientRect().width - 12
          if (measured > 0) setAvailableWidth(measured)
          drag.current = {
            pointerId: event.pointerId,
            x: event.clientX,
            width: panelWidth,
          }
          setDragging(true)
        }}
        onPointerMove={(event) => {
          const start = drag.current
          if (!start || start.pointerId !== event.pointerId) return
          setWidth(
            Math.max(
              minimumWidth,
              Math.min(maximumWidth, start.width + start.x - event.clientX),
            ),
          )
        }}
        onPointerUp={(event) => {
          if (drag.current?.pointerId !== event.pointerId) return
          event.currentTarget.releasePointerCapture(event.pointerId)
          drag.current = null
          setDragging(false)
        }}
        onPointerCancel={() => {
          drag.current = null
          setDragging(false)
        }}
        onLostPointerCapture={() => {
          drag.current = null
          setDragging(false)
        }}
        onDoubleClick={() => setWidth(defaultWidth)}
        onKeyDown={(event) => {
          const step = event.shiftKey ? 64 : 32
          let next: number
          switch (event.key) {
            case 'ArrowLeft':
              next = panelWidth + step
              break
            case 'ArrowRight':
              next = panelWidth - step
              break
            case 'Home':
              next = minimumWidth
              break
            case 'End':
              next = maximumWidth
              break
            default:
              return
          }
          event.preventDefault()
          setWidth(Math.max(minimumWidth, Math.min(maximumWidth, next)))
        }}
      />
      <aside
        id="training-chat-panel"
        className={styles.panel}
        aria-label={t('training.chatTitle')}
      >
        {chat}
      </aside>
    </main>
  )
}
