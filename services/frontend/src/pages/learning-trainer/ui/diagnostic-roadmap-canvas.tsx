import { Badge, Heading, IconButton } from '@radix-ui/themes'
import {
  Background,
  BackgroundVariant,
  Handle,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  CircleDot,
  Compass,
  Maximize2,
  Minimize2,
  Sparkles,
  X,
  ZoomIn,
  ZoomOut,
} from 'lucide-react'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from 'react'
import { useTranslation } from 'react-i18next'
import type { CompetencyMapOutline } from '@/entities/learning-competency-map'
import {
  layoutDiagnosticMindmap,
  type CompetencyNodeData,
  type ConstituentNodeData,
  type DiagnosticMindmapNode,
  type SubjectNodeData,
} from '../model/diagnostic-mindmap'
import styles from './diagnostic-roadmap-canvas.module.scss'

type SelectionContextType = {
  selectedId: string | null
  setSelectedId: (id: string | null) => void
}

const SelectionContext = createContext<SelectionContextType>({
  selectedId: null,
  setSelectedId: () => {},
})

function SubjectNodeCard({ data }: NodeProps<DiagnosticMindmapNode>) {
  const subject = data as SubjectNodeData
  return (
    <div
      className={styles.subjectNode}
      role="region"
      aria-label={subject.title}
    >
      <span className={styles.subjectTitle} title={subject.title}>
        {subject.title}
      </span>
      <div className={styles.subjectCircle}>
        <Handle
          type="source"
          position={Position.Left}
          id="source-left"
          className={styles.handle}
        />
        <Handle
          type="source"
          position={Position.Right}
          id="source-right"
          className={styles.handle}
        />
        <Compass size={28} className={styles.subjectIcon} aria-hidden="true" />
      </div>
    </div>
  )
}

function CompetencyNodeCard({ data }: NodeProps<DiagnosticMindmapNode>) {
  const comp = data as CompetencyNodeData
  const isRight = comp.side === 'right'

  const cssVars = {
    '--branch-color': comp.color,
    '--branch-border': comp.border,
    '--branch-glow': comp.glow,
    '--branch-bg': comp.lightBg,
    '--branch-accent': comp.accent,
  } as CSSProperties

  const displayTitle = comp.cleanTitle || comp.title

  return (
    <div
      className={styles.competencyNode}
      style={cssVars}
      role="region"
      aria-label={displayTitle}
    >
      <span className={styles.competencyTitle} title={comp.title}>
        {displayTitle}
      </span>
      <div className={styles.competencyCircle}>
        {isRight ? (
          <>
            <Handle
              type="target"
              position={Position.Left}
              id="target-left"
              className={styles.handle}
            />
            <Handle
              type="source"
              position={Position.Right}
              id="source-right"
              className={styles.handle}
            />
          </>
        ) : (
          <>
            <Handle
              type="target"
              position={Position.Right}
              id="target-right"
              className={styles.handle}
            />
            <Handle
              type="source"
              position={Position.Left}
              id="source-left"
              className={styles.handle}
            />
          </>
        )}
        <Sparkles
          size={22}
          className={styles.competencyIcon}
          aria-hidden="true"
        />
      </div>
    </div>
  )
}

function ConstituentNodeCard({ data }: NodeProps<DiagnosticMindmapNode>) {
  const { selectedId, setSelectedId } = useContext(SelectionContext)
  const item = data as ConstituentNodeData
  const isRight = item.side === 'right'
  const isSelected = selectedId === item.id

  const cssVars = {
    '--branch-color': item.color,
    '--branch-border': item.border,
    '--branch-glow': item.glow,
    '--branch-bg': item.lightBg,
    '--branch-accent': item.accent,
  } as CSSProperties

  const handleClick = useCallback(() => {
    setSelectedId(isSelected ? null : item.id)
  }, [isSelected, item.id, setSelectedId])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        handleClick()
      }
    },
    [handleClick],
  )

  const displayTitle = item.cleanTitle || item.title

  return (
    <div
      className={styles.constituentNode}
      style={cssVars}
      data-side={item.side}
      data-selected={isSelected}
      role="button"
      tabIndex={0}
      aria-label={displayTitle}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      <span className={styles.constituentTitle} title={item.title}>
        {displayTitle}
      </span>
      <div className={styles.constituentCircle}>
        {isRight ? (
          <Handle
            type="target"
            position={Position.Left}
            id="target-left"
            className={styles.handle}
          />
        ) : (
          <Handle
            type="target"
            position={Position.Right}
            id="target-right"
            className={styles.handle}
          />
        )}
        <CircleDot
          size={20}
          className={styles.constituentIcon}
          aria-hidden="true"
        />
      </div>
    </div>
  )
}

const nodeTypes = {
  subject: SubjectNodeCard,
  competency: CompetencyNodeCard,
  constituent: ConstituentNodeCard,
}

export default function DiagnosticRoadmapCanvas({
  outline,
  subjectTitle,
}: {
  outline: CompetencyMapOutline
  subjectTitle: string
}) {
  return (
    <ReactFlowProvider>
      <DiagnosticMindmapExplorer
        outline={outline}
        subjectTitle={subjectTitle}
      />
    </ReactFlowProvider>
  )
}

function DiagnosticMindmapExplorer({
  outline,
  subjectTitle,
}: {
  outline: CompetencyMapOutline
  subjectTitle: string
}) {
  const { t } = useTranslation()
  const flow = useReactFlow()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const { nodes: initialNodes, edges: initialEdges } = useMemo(
    () => layoutDiagnosticMindmap(outline, subjectTitle, selectedId),
    [outline, subjectTitle, selectedId],
  )

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)

  useEffect(() => {
    setNodes(initialNodes)
    setEdges(initialEdges)
  }, [initialNodes, initialEdges, setNodes, setEdges])

  const fit = useCallback(() => {
    window.requestAnimationFrame(() => {
      try {
        void flow.fitView({ padding: 0.18, duration: 350 })
      } catch {
        // Safe in non-DOM test environments
      }
    })
  }, [flow])

  useEffect(() => {
    fit()
  }, [outline, fit])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const handleWheel = (e: WheelEvent) => {
      const target = e.target as HTMLElement | null
      if (target?.closest(`.${styles.detailsOutcomes}`)) {
        return
      }

      e.preventDefault()

      try {
        const { x, y, zoom } = flow.getViewport()

        let delta = e.deltaY
        if (e.deltaMode === 1) {
          delta *= 33
        } else if (e.deltaMode === 2) {
          delta *= 500
        }

        // Snappy, accelerated zoom factor
        const zoomMultiplier = Math.min(
          2.0,
          Math.max(0.5, Math.exp(-delta * 0.0035)),
        )
        const minZoom = 0.15
        const maxZoom = 3.0
        const nextZoom = Math.min(
          Math.max(zoom * zoomMultiplier, minZoom),
          maxZoom,
        )

        if (Math.abs(nextZoom - zoom) < 0.001) return

        const rect = container.getBoundingClientRect()
        const mouseX = e.clientX - rect.left
        const mouseY = e.clientY - rect.top

        const nextX = mouseX - (mouseX - x) * (nextZoom / zoom)
        const nextY = mouseY - (mouseY - y) * (nextZoom / zoom)

        flow.setViewport({ x: nextX, y: nextY, zoom: nextZoom })
      } catch {
        // Safe fallback in test or detached DOM environments
      }
    }

    container.addEventListener('wheel', handleWheel, { passive: false })
    return () => {
      container.removeEventListener('wheel', handleWheel)
    }
  }, [flow])

  const selectedConstituent = useMemo(() => {
    if (!selectedId) return null
    for (const comp of outline.competencies) {
      for (const item of comp.constituents) {
        if (item.id === selectedId) {
          return {
            constituent: item,
            competency: comp,
          }
        }
      }
    }
    return null
  }, [selectedId, outline])

  const contextValue = useMemo(
    () => ({
      selectedId,
      setSelectedId,
    }),
    [selectedId],
  )

  return (
    <SelectionContext.Provider value={contextValue}>
      <div
        ref={containerRef}
        className={styles.wrapper}
        data-fullscreen={isFullscreen}
      >
        <span className={styles.hint} aria-hidden="true">
          <Sparkles size={14} />
          {t(
            'learningRoadmap.selectHint',
            'Нажмите на составляющую, чтобы увидеть результаты для проверки',
          )}
        </span>

        <div className={styles.toolbar} role="toolbar">
          <IconButton
            size="2"
            variant="ghost"
            onClick={() => void flow.zoomIn({ duration: 250 })}
            aria-label={t('learningRoadmap.zoomIn', 'Приблизить')}
          >
            <ZoomIn size={16} />
          </IconButton>
          <IconButton
            size="2"
            variant="ghost"
            onClick={() => void flow.zoomOut({ duration: 250 })}
            aria-label={t('learningRoadmap.zoomOut', 'Отдалить')}
          >
            <ZoomOut size={16} />
          </IconButton>
          <IconButton
            size="2"
            variant="ghost"
            onClick={fit}
            aria-label={t('learningRoadmap.fitView', 'По центру')}
          >
            <Maximize2 size={16} />
          </IconButton>
          <IconButton
            size="2"
            variant="ghost"
            onClick={() => setIsFullscreen((prev) => !prev)}
            aria-label={
              isFullscreen
                ? t('learningRoadmap.exitFullscreen', 'Свернуть')
                : t('learningRoadmap.fullscreen', 'Во весь экран')
            }
          >
            {isFullscreen ? <Minimize2 size={16} /> : <Maximize2 size={16} />}
          </IconButton>
        </div>

        <div className={styles.canvasContainer}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onPaneClick={() => setSelectedId(null)}
            fitView
            zoomOnScroll={false}
            minZoom={0.15}
            maxZoom={3.0}
            proOptions={{ hideAttribution: true }}
          >
            <Background
              variant={BackgroundVariant.Dots}
              gap={20}
              size={1.5}
              color="var(--gray-a4)"
            />
          </ReactFlow>
        </div>

        {selectedConstituent && (
          <aside
            className={styles.detailsPanel}
            aria-label={selectedConstituent.constituent.name}
          >
            <div className={styles.detailsHeader}>
              <div>
                <span className={styles.detailsCompetency}>
                  <span className={styles.detailsDot} />
                  {selectedConstituent.competency.name}
                </span>
                <Heading as="h3" size="3" className={styles.constituentTitle}>
                  {selectedConstituent.constituent.name}
                </Heading>
              </div>
              <IconButton
                size="1"
                variant="ghost"
                onClick={() => setSelectedId(null)}
                aria-label={t('common.close', 'Закрыть')}
              >
                <X size={14} />
              </IconButton>
            </div>
            <div className={styles.detailsOutcomes}>
              {selectedConstituent.constituent.outcomes.map((outcome) => (
                <div key={outcome.id} className={styles.detailsOutcomeItem}>
                  {outcome.role && (
                    <Badge
                      size="1"
                      variant={outcome.role === 'main' ? 'solid' : 'soft'}
                      color={outcome.role === 'main' ? 'indigo' : 'gray'}
                    >
                      {outcome.role === 'main'
                        ? t('learningRoadmap.mainOutcome', 'Основной')
                        : t('learningRoadmap.basicOutcome', 'Базовый')}
                    </Badge>
                  )}
                  <span>{outcome.name}</span>
                </div>
              ))}
            </div>
          </aside>
        )}
      </div>
    </SelectionContext.Provider>
  )
}
