import {
  Badge,
  Button,
  Heading,
  IconButton,
  Switch,
  Text,
  TextField,
} from '@radix-ui/themes'
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useNodesState,
  useReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  ArrowLeftToLine,
  ArrowRightFromLine,
  Layers,
  Maximize2,
  Minimize2,
  Search,
  X,
} from 'lucide-react'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import type { CourseGraph } from '@/entities/competency-map'
import { MarkdownContent } from '@/shared/ui'
import {
  NODE_WIDTH,
  buildCourseGraphView,
  layoutCourseGraph,
  neighbourhood,
  searchTopics,
  type CourseGraphView,
  type GraphLink,
  type GraphSection,
  type GraphTopicView,
} from '../lib/course-graph-view'
import styles from './course-graph-canvas.module.scss'

type TopicNode = Node<{ view: GraphTopicView }, 'topic'>
type ClusterNode = Node<
  { section: GraphSection; members: string[]; framed: boolean },
  'cluster'
>
type GraphNode = TopicNode | ClusterNode

// What the canvas emphasizes: null activeIds keeps every node at full strength.
type Highlight = {
  selectedId: string | null
  focusId: string | null
  activeIds: Set<string> | null
}

const HighlightContext = createContext<Highlight>({
  selectedId: null,
  focusId: null,
  activeIds: null,
})

const nodeTypes = { topic: TopicNodeCard, cluster: ClusterFrame }

const SEARCH_RESULTS_LIMIT = 8
const FIT_PADDING = 0.08
// The details card covers the right side of a wide canvas.
const DETAILS_WIDTH = 360
const DETAILS_OVERLAY_MIN = 720

export default function CourseGraphCanvas({
  graph,
  materialNames,
}: {
  graph: CourseGraph
  // Material id → file name, for the sources of a topic.
  materialNames: Map<string, string>
}) {
  return (
    <ReactFlowProvider>
      <GraphExplorer graph={graph} materialNames={materialNames} />
    </ReactFlowProvider>
  )
}

function GraphExplorer({
  graph,
  materialNames,
}: {
  graph: CourseGraph
  materialNames: Map<string, string>
}) {
  const { t } = useTranslation()
  const flow = useReactFlow<GraphNode>()
  const canvasRef = useRef<HTMLDivElement>(null)
  const view = useMemo(() => buildCourseGraphView(graph), [graph])
  const initialNodes = useMemo(() => toNodes(view), [view])
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  useEffect(() => setNodes(initialNodes), [initialNodes, setNodes])

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [hoveredId, setHoveredId] = useState<string | null>(null)
  const [sectionId, setSectionId] = useState<string | null | undefined>()
  const [query, setQuery] = useState('')
  const [showHierarchy, setShowHierarchy] = useState(true)
  const [expanded, setExpanded] = useState(false)

  const matches = useMemo(() => searchTopics(view, query), [view, query])
  const selected = selectedId ? view.topics.get(selectedId) : undefined

  const highlight = useMemo<Highlight>(() => {
    const focusId = selectedId ?? hoveredId
    if (focusId && view.topics.has(focusId))
      return { selectedId, focusId, activeIds: neighbourhood(view, focusId) }
    if (query.trim())
      return { selectedId, focusId: null, activeIds: new Set(matches) }
    if (sectionId !== undefined)
      return {
        selectedId,
        focusId: null,
        activeIds: new Set(
          view.order.filter(
            (id) => view.topics.get(id)!.section.id === sectionId,
          ),
        ),
      }
    return { selectedId, focusId: null, activeIds: null }
  }, [view, selectedId, hoveredId, query, matches, sectionId])

  const edges = useMemo(
    () => toEdges(view, highlight, showHierarchy),
    [view, highlight, showHierarchy],
  )

  const focusTopic = useCallback(
    (id: string) => {
      setSelectedId(id)
      const node = flow.getInternalNode(id)
      if (!node) return
      const { x, y } = node.internals.positionAbsolute
      const zoom = Math.max(flow.getZoom(), 0.9)
      const covered =
        (canvasRef.current?.clientWidth ?? 0) >= DETAILS_OVERLAY_MIN
          ? DETAILS_WIDTH / 2 / zoom
          : 0
      void flow.setCenter(
        x + (node.measured.width ?? NODE_WIDTH) / 2 + covered,
        y + (node.measured.height ?? 80) / 2,
        { zoom, duration: 400 },
      )
    },
    [flow],
  )

  const toggleSection = (section: GraphSection) => {
    const next = sectionId === section.id ? undefined : section.id
    setSectionId(next)
    // fitView waits for the next node change; bounds move the camera now.
    const cluster = flow.getNode(clusterId(section))
    const bounds =
      next !== undefined && cluster
        ? flow.getNodesBounds([cluster])
        : flow.getNodesBounds(flow.getNodes())
    void flow.fitBounds(bounds, { padding: FIT_PADDING, duration: 400 })
  }

  // A new layout or a resized canvas needs the whole graph in view again.
  useEffect(() => {
    const frame = requestAnimationFrame(
      () => void flow.fitView({ padding: FIT_PADDING, duration: 300 }),
    )
    return () => cancelAnimationFrame(frame)
  }, [flow, expanded, initialNodes])

  useEffect(() => {
    if (!expanded) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setExpanded(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [expanded])

  return (
    <div className={styles.explorer} data-expanded={expanded}>
      <div className={styles.toolbar}>
        <TextField.Root
          className={styles.search}
          size="2"
          value={query}
          placeholder={t('competencyMap.graphView.searchPlaceholder')}
          aria-label={t('competencyMap.graphView.search')}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && matches[0]) focusTopic(matches[0])
          }}
        >
          <TextField.Slot>
            <Search size={16} aria-hidden="true" />
          </TextField.Slot>
          {query && (
            <TextField.Slot>
              <IconButton
                size="1"
                variant="ghost"
                color="gray"
                aria-label={t('competencyMap.graphView.clearSearch')}
                onClick={() => setQuery('')}
              >
                <X size={14} aria-hidden="true" />
              </IconButton>
            </TextField.Slot>
          )}
        </TextField.Root>
        <Text as="label" size="2" className={styles.toggle}>
          <Switch
            size="1"
            checked={showHierarchy}
            onCheckedChange={setShowHierarchy}
          />
          {t('competencyMap.graphView.showHierarchy')}
        </Text>
        <Button
          size="2"
          variant="soft"
          color="gray"
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? (
            <Minimize2 size={16} aria-hidden="true" />
          ) : (
            <Maximize2 size={16} aria-hidden="true" />
          )}
          {expanded
            ? t('competencyMap.graphView.collapse')
            : t('competencyMap.graphView.expand')}
        </Button>
      </div>

      {query.trim() && (
        <div className={styles.searchResults}>
          <Text size="1" color="gray" role="status">
            {matches.length > 0
              ? t('competencyMap.graphView.found', { count: matches.length })
              : t('competencyMap.graphView.notFound')}
          </Text>
          {matches.slice(0, SEARCH_RESULTS_LIMIT).map((id) => (
            <button
              key={id}
              type="button"
              className={styles.chip}
              data-tone={view.topics.get(id)!.section.tone}
              onClick={() => focusTopic(id)}
            >
              {view.topics.get(id)!.topic.title}
            </button>
          ))}
        </div>
      )}

      {view.sections.length > 1 && (
        <div
          className={styles.legend}
          role="group"
          aria-label={t('competencyMap.graphView.sections')}
        >
          {view.sections.map((section) => (
            <button
              key={clusterId(section)}
              type="button"
              className={styles.chip}
              data-tone={section.tone}
              aria-pressed={sectionId === section.id}
              onClick={() => toggleSection(section)}
            >
              <span className={styles.swatch} aria-hidden="true" />
              {section.title ?? t('competencyMap.graphView.looseTopics')}
              <span className={styles.chipCount}>{section.size}</span>
            </button>
          ))}
        </div>
      )}

      <div className={styles.stage}>
        <div
          ref={canvasRef}
          className={styles.canvas}
          role="region"
          aria-label={t('competencyMap.graphView.canvas')}
        >
          <HighlightContext.Provider value={highlight}>
            <ReactFlow<GraphNode>
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onNodesChange={onNodesChange}
              onNodeClick={(_, node) => {
                if (node.type === 'topic') focusTopic(node.id)
              }}
              onNodeMouseEnter={(_, node) => {
                if (node.type === 'topic') setHoveredId(node.id)
              }}
              onNodeMouseLeave={() => setHoveredId(null)}
              onPaneClick={() => setSelectedId(null)}
              // A focused card opens with Enter or Space like a button.
              onKeyDown={(event) => {
                if (event.key !== 'Enter' && event.key !== ' ') return
                const id = (event.target as HTMLElement)
                  .closest('.react-flow__node-topic')
                  ?.getAttribute('data-id')
                if (!id) return
                event.preventDefault()
                focusTopic(id)
              }}
              elementsSelectable={false}
              nodesConnectable={false}
              edgesFocusable={false}
              minZoom={0.05}
              maxZoom={2}
              fitView
              fitViewOptions={{ padding: FIT_PADDING }}
            >
              <Background
                variant={BackgroundVariant.Dots}
                gap={20}
                size={1.2}
                color="var(--gray-a6)"
              />
              <Controls
                showInteractive={false}
                aria-label={t('competencyMap.graphView.controls')}
              />
              <MiniMap<GraphNode>
                pannable
                zoomable
                ariaLabel={t('competencyMap.graphView.minimap')}
                className={styles.minimap}
                nodeClassName={(node) =>
                  node.type === 'topic'
                    ? (styles[`minimap-${node.data.view.section.tone}`] ?? '')
                    : styles.minimapCluster
                }
              />
            </ReactFlow>
          </HighlightContext.Provider>
        </div>

        {selected && (
          <TopicDetails
            view={view}
            topic={selected}
            materialNames={materialNames}
            onOpen={focusTopic}
            onClose={() => setSelectedId(null)}
          />
        )}
      </div>
    </div>
  )
}

function clusterId(section: GraphSection) {
  return `cluster:${section.id ?? ''}`
}

// Clusters come first: React Flow needs a parent before its children, whose
// positions are relative to it so an island drags as a whole.
function toNodes(view: CourseGraphView): GraphNode[] {
  const { positions, clusters } = layoutCourseGraph(view)
  const framed = clusters.length > 1 || clusters[0]?.section.id != null
  const result: GraphNode[] = clusters.map((cluster) => ({
    id: clusterId(cluster.section),
    type: 'cluster',
    position: { x: cluster.x, y: cluster.y },
    width: cluster.width,
    height: cluster.height,
    data: {
      section: cluster.section,
      members: view.order.filter(
        (id) => view.topics.get(id)!.section === cluster.section,
      ),
      framed,
    },
    focusable: false,
    zIndex: -1,
  }))
  const byId = new Map(clusters.map((c) => [clusterId(c.section), c]))
  for (const id of view.order) {
    const topic = view.topics.get(id)!
    const parentId = clusterId(topic.section)
    const cluster = byId.get(parentId)!
    const position = positions.get(id) ?? { x: cluster.x, y: cluster.y }
    result.push({
      id,
      type: 'topic',
      parentId,
      position: { x: position.x - cluster.x, y: position.y - cluster.y },
      data: { view: topic },
      ariaLabel: topic.topic.title,
    })
  }
  return result
}

function toEdges(
  view: CourseGraphView,
  { focusId, activeIds }: Highlight,
  showHierarchy: boolean,
): Edge[] {
  const stateOf = (source: string, target: string) => {
    if (!activeIds) return 'idle'
    const linked = activeIds.has(source) && activeIds.has(target)
    if (focusId)
      return linked && (source === focusId || target === focusId)
        ? 'active'
        : 'dimmed'
    return linked ? 'active' : 'dimmed'
  }
  const hierarchy: Edge[] = view.hierarchyEdges.map((edge) => {
    const state = stateOf(edge.source, edge.target)
    return {
      ...edge,
      type: 'smoothstep',
      className: `${styles.hierarchyEdge} ${styles[`edge-${state}`] ?? ''}`,
      hidden: !showHierarchy,
    }
  })
  const prerequisites: Edge[] = view.prerequisiteEdges.map((edge) => {
    const state = stateOf(edge.source, edge.target)
    return {
      ...edge,
      className: [
        styles.prerequisiteEdge,
        styles[`edge-${state}`],
        view.topics.get(edge.source)!.section !==
          view.topics.get(edge.target)!.section && styles.crossEdge,
      ]
        .filter(Boolean)
        .join(' '),
      animated: state === 'active' && focusId !== null,
      zIndex: state === 'active' ? 1 : 0,
      markerEnd: {
        type: MarkerType.ArrowClosed,
        width: 16,
        height: 16,
        color: state === 'active' ? 'var(--accent-9)' : 'var(--gray-8)',
      },
    }
  })
  return [...hierarchy, ...prerequisites]
}

function ClusterFrame({ data }: NodeProps<ClusterNode>) {
  const { t } = useTranslation()
  const { activeIds } = useContext(HighlightContext)
  const { section, members, framed } = data
  const dimmed = activeIds !== null && !members.some((id) => activeIds.has(id))
  return (
    <div
      className={styles.cluster}
      data-tone={section.tone}
      data-framed={framed}
      data-dimmed={dimmed}
    >
      {framed && (
        <span className={styles.clusterTitle}>
          {section.title ?? t('competencyMap.graphView.looseTopics')}
          <span className={styles.chipCount}>{section.size}</span>
        </span>
      )}
    </div>
  )
}

function TopicNodeCard({ id, data }: NodeProps<TopicNode>) {
  const { t } = useTranslation()
  const { selectedId, focusId, activeIds } = useContext(HighlightContext)
  const { view } = data
  const state =
    id === selectedId
      ? 'selected'
      : !activeIds
        ? 'idle'
        : id === focusId
          ? 'focus'
          : activeIds.has(id)
            ? 'related'
            : 'dimmed'
  return (
    <div
      className={styles.node}
      data-tone={view.section.tone}
      data-kind={view.isSection ? 'section' : 'topic'}
      data-state={state}
    >
      <Handle
        type="target"
        position={Position.Left}
        className={styles.handle}
        isConnectable={false}
      />
      {view.isSection && (
        <span className={styles.nodeEyebrow}>
          {t('competencyMap.graphView.section')}
        </span>
      )}
      <span className={styles.nodeTitle}>{view.topic.title}</span>
      <span className={styles.nodeMeta}>
        <span className={styles.srOnly}>
          {t('competencyMap.graphView.nodeSummary', {
            prerequisites: uniqueCount(view.prerequisites),
            dependents: uniqueCount(view.dependents),
          })}
        </span>
        {view.prerequisites.length > 0 && (
          <span className={styles.metaItem} aria-hidden="true">
            <ArrowLeftToLine size={12} />
            {uniqueCount(view.prerequisites)}
          </span>
        )}
        {view.dependents.length > 0 && (
          <span className={styles.metaItem} aria-hidden="true">
            <ArrowRightFromLine size={12} />
            {uniqueCount(view.dependents)}
          </span>
        )}
        {view.childIds.length > 0 && (
          <span className={styles.metaItem} aria-hidden="true">
            <Layers size={12} />
            {view.childIds.length}
          </span>
        )}
      </span>
      <Handle
        type="source"
        position={Position.Right}
        className={styles.handle}
        isConnectable={false}
      />
    </div>
  )
}

function uniqueCount(links: GraphLink[]) {
  return new Set(links.map((link) => link.id)).size
}

// Several requirements can link the same pair of topics; the list shows the
// topic once with every reason under it.
function groupLinks(links: GraphLink[]) {
  const grouped = new Map<string, string[]>()
  for (const link of links) {
    const reasons = grouped.get(link.id) ?? []
    const reason = link.reason || link.requirement
    if (reason && !reasons.includes(reason)) reasons.push(reason)
    grouped.set(link.id, reasons)
  }
  return [...grouped]
}

function TopicDetails({
  view,
  topic,
  materialNames,
  onOpen,
  onClose,
}: {
  view: CourseGraphView
  topic: GraphTopicView
  materialNames: Map<string, string>
  onOpen: (id: string) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [theoryOpen, setTheoryOpen] = useState(false)
  useEffect(() => setTheoryOpen(false), [topic])
  const titleOf = (id: string) => view.topics.get(id)?.topic.title ?? id
  const toneOf = (id: string) => view.topics.get(id)?.section.tone ?? 'gray'
  const { provides = [], requires = [], sources = [] } = topic.topic
  const parent = topic.parentId

  const topicButton = (id: string) => (
    <button
      type="button"
      className={styles.chip}
      data-tone={toneOf(id)}
      onClick={() => onOpen(id)}
    >
      {titleOf(id)}
    </button>
  )

  return (
    <aside
      className={styles.details}
      aria-labelledby="course-graph-topic-title"
    >
      <div className={styles.detailsHeader}>
        <Badge
          color={topic.section.tone}
          variant="soft"
          className={styles.detailsSection}
        >
          {topic.isSection
            ? t('competencyMap.graphView.section')
            : (topic.section.title ?? t('competencyMap.graphView.looseTopics'))}
        </Badge>
        <IconButton
          size="1"
          variant="ghost"
          color="gray"
          aria-label={t('competencyMap.graphView.closeDetails')}
          onClick={onClose}
        >
          <X size={16} aria-hidden="true" />
        </IconButton>
      </div>
      <Heading as="h3" size="3" id="course-graph-topic-title">
        {topic.topic.title}
      </Heading>

      {topic.topic.question && (
        <DetailsBlock title={t('competencyMap.graphView.question')}>
          <Text as="p" size="2" className={styles.question}>
            {topic.topic.question}
          </Text>
        </DetailsBlock>
      )}

      {topic.topic.text?.trim() && (
        <DetailsBlock title={t('competencyMap.graphView.theory')}>
          <div className={styles.theory} data-open={theoryOpen}>
            <MarkdownContent text={topic.topic.text} />
          </div>
          <Button
            size="1"
            variant="ghost"
            onClick={() => setTheoryOpen((value) => !value)}
          >
            {theoryOpen
              ? t('competencyMap.graphView.theoryLess')
              : t('competencyMap.graphView.theoryMore')}
          </Button>
        </DetailsBlock>
      )}

      {topic.prerequisites.length > 0 && (
        <DetailsBlock title={t('competencyMap.graphView.prerequisites')}>
          <ul className={styles.linkList}>
            {groupLinks(topic.prerequisites).map(([id, reasons]) => (
              <li key={id}>
                {topicButton(id)}
                {reasons.map((reason) => (
                  <Text key={reason} as="p" size="1" color="gray">
                    {reason}
                  </Text>
                ))}
              </li>
            ))}
          </ul>
        </DetailsBlock>
      )}

      {topic.dependents.length > 0 && (
        <DetailsBlock title={t('competencyMap.graphView.dependents')}>
          <ul className={styles.linkList}>
            {groupLinks(topic.dependents).map(([id]) => (
              <li key={id}>{topicButton(id)}</li>
            ))}
          </ul>
        </DetailsBlock>
      )}

      {(parent || topic.childIds.length > 0) && (
        <DetailsBlock title={t('competencyMap.graphView.structure')}>
          <ul className={styles.linkList}>
            {parent && (
              <li>
                <Text as="p" size="1" color="gray">
                  {t('competencyMap.graphView.parent')}
                </Text>
                {topicButton(parent)}
              </li>
            )}
            {topic.childIds.length > 0 && (
              <li>
                <Text as="p" size="1" color="gray">
                  {t('competencyMap.graphView.children')}
                </Text>
                <div className={styles.chips}>
                  {topic.childIds.map((id) => (
                    <span key={id}>{topicButton(id)}</span>
                  ))}
                </div>
              </li>
            )}
          </ul>
        </DetailsBlock>
      )}

      {provides.length > 0 && (
        <DetailsBlock title={t('competencyMap.graphView.provides')}>
          <ul className={styles.bullets}>
            {provides.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </DetailsBlock>
      )}

      {requires.length > 0 && (
        <DetailsBlock title={t('competencyMap.graphView.requires')}>
          <ul className={styles.bullets}>
            {requires.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </DetailsBlock>
      )}

      {sources.length > 0 && (
        <DetailsBlock title={t('competencyMap.graphView.sources')}>
          <ul className={styles.bullets}>
            {uniqueSources(sources).map((source) => (
              <li key={source.key}>
                {[
                  materialNames.get(source.material_id) ?? source.material_id,
                  source.week != null &&
                    t('competencyMap.graphView.week', { week: source.week }),
                  source.page != null &&
                    t('competencyMap.graphView.page', { page: source.page }),
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </li>
            ))}
          </ul>
        </DetailsBlock>
      )}
    </aside>
  )
}

function uniqueSources(
  sources: NonNullable<GraphTopicView['topic']['sources']>,
) {
  const seen = new Map<string, (typeof sources)[number] & { key: string }>()
  for (const source of sources) {
    const key = `${source.material_id}:${source.week ?? ''}:${source.page ?? ''}`
    if (!seen.has(key)) seen.set(key, { ...source, key })
  }
  return [...seen.values()]
}

function DetailsBlock({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className={styles.detailsBlock}>
      <Text as="p" size="1" weight="bold" className={styles.detailsLabel}>
        {title}
      </Text>
      {children}
    </section>
  )
}
