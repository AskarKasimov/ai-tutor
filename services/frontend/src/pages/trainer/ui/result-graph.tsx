import { Heading, Text } from '@radix-ui/themes'
import { type CSSProperties, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { TopicResult, TopicStatus } from '@/entities/diagnostic-session'
import { layoutResultGraph } from '../lib/result-graph'
import styles from './geek-result.module.scss'

// Card and spacing sizes of the map, in pixels.
const NODE_WIDTH = 208
const NODE_HEIGHT = 92
const COLUMN_GAP = 64
const ROW_GAP = 20
const PADDING = 28

const legendOrder: TopicStatus[] = [
  'independent',
  'credited_by_parent',
  'assisted',
  'gap',
  'insufficient',
]

// The checked part of the course graph: which topic relies on which, with
// the result of every topic. Prerequisites stand left of the topics on them.
export function ResultGraph({ topics }: { topics: TopicResult[] }) {
  const { t } = useTranslation()
  const graph = useMemo(() => layoutResultGraph(topics), [topics])
  if (graph.nodes.length === 0) return null

  const x = (column: number) => PADDING + column * (NODE_WIDTH + COLUMN_GAP)
  const y = (row: number) => PADDING + row * (NODE_HEIGHT + ROW_GAP)
  const width = x(graph.columns) - COLUMN_GAP + PADDING
  const height = y(graph.rows) - ROW_GAP + PADDING
  const place = new Map(graph.nodes.map((node) => [node.topic.nodeId, node]))
  const statuses = legendOrder.filter((status) =>
    graph.nodes.some((node) => node.topic.status === status),
  )

  return (
    <section aria-labelledby="graph-title" className={styles.panel}>
      <Heading as="h2" size="5" id="graph-title">
        {t('geek.graphTitle')}
      </Heading>
      <Text as="p" className={styles.lead}>
        {t('geek.graphHint')}
      </Text>
      <ul className={styles.legend} aria-label={t('geek.graphLegend')}>
        {statuses.map((status) => (
          <li key={status} data-status={status}>
            {t(`geek.status.${status}`)}
          </li>
        ))}
      </ul>
      {/* A wide map scrolls sideways; the region is focusable for keyboards. */}
      <div
        className={styles.graphScroll}
        role="region"
        aria-label={t('geek.graphScroll')}
        tabIndex={0}
      >
        <div
          className={styles.graphCanvas}
          style={{ width, height } as CSSProperties}
        >
          <svg
            className={styles.graphEdges}
            width={width}
            height={height}
            aria-hidden="true"
          >
            <defs>
              <marker
                id="graph-arrow"
                viewBox="0 0 10 10"
                refX="9"
                refY="5"
                markerWidth="7"
                markerHeight="7"
                orient="auto-start-reverse"
              >
                <path d="M0,0 L10,5 L0,10 z" className={styles.graphArrow} />
              </marker>
              {graph.edges.map((edge, index) => {
                const from = place.get(edge.from)!
                const to = place.get(edge.to)!
                return (
                  <linearGradient
                    key={index}
                    id={`graph-edge-${index}`}
                    gradientUnits="userSpaceOnUse"
                    x1={x(from.column) + NODE_WIDTH}
                    x2={x(to.column)}
                    y1={0}
                    y2={0}
                  >
                    <stop offset="0%" className={styles.edgeStart} />
                    <stop offset="50%" className={styles.edgeMiddle} />
                    <stop offset="100%" className={styles.edgeEnd} />
                  </linearGradient>
                )
              })}
            </defs>
            {graph.edges.map((edge, index) => {
              const from = place.get(edge.from)!
              const to = place.get(edge.to)!
              // The edge leaves the right side of the prerequisite, runs
              // straight through its lane slots and enters the topic's left side.
              const middle = (row: number) => y(row) + NODE_HEIGHT / 2
              const points = [
                [x(from.column) + NODE_WIDTH, middle(from.row)],
                ...edge.via.flatMap(({ column, row }) => [
                  [x(column), middle(row)],
                  [x(column) + NODE_WIDTH, middle(row)],
                ]),
                [x(to.column) - 4, middle(to.row)],
              ]
              let d = `M${points[0][0]},${points[0][1]}`
              for (let i = 1; i < points.length; i++) {
                const [ax, ay] = points[i - 1]
                const [bx, by] = points[i]
                // Odd steps cross a column gap with a curve; even ones run along a lane.
                if (i % 2 === 1) {
                  const bend = (bx - ax) / 2
                  d += ` C${ax + bend},${ay} ${bx - bend},${by} ${bx},${by}`
                } else d += ` L${bx},${by}`
              }
              return (
                <path
                  key={index}
                  d={d}
                  className={styles.graphEdge}
                  data-gap={from.topic.status === 'gap' || undefined}
                  stroke={`url(#graph-edge-${index})`}
                  markerEnd="url(#graph-arrow)"
                />
              )
            })}
          </svg>
          <ol className={styles.graphNodes} aria-label={t('geek.graphNodes')}>
            {graph.nodes.map(({ topic, column, row }) => {
              const requires = graph.edges
                .filter((edge) => edge.to === topic.nodeId)
                .map((edge) => place.get(edge.from)!.topic.title)
              return (
                <li
                  key={topic.nodeId}
                  className={styles.graphNode}
                  data-status={topic.status}
                  data-top={topic.topLevel || undefined}
                  style={
                    {
                      left: x(column),
                      top: y(row),
                      width: NODE_WIDTH,
                      height: NODE_HEIGHT,
                    } as CSSProperties
                  }
                >
                  <span className={styles.graphNodeTitle} title={topic.title}>
                    {topic.title}
                  </span>
                  <span className={styles.graphNodeMeta}>
                    <span
                      className={styles.status}
                      title={t(`geek.status.${topic.status}`)}
                    >
                      {t(`geek.statusShort.${topic.status}`)}
                    </span>
                    <span className={styles.graphGrade}>
                      {topic.grade ?? '—'}
                    </span>
                  </span>
                  {requires.length > 0 && (
                    <span className={styles.visuallyHidden}>
                      {t('geek.graphRequires', { titles: requires.join(', ') })}
                    </span>
                  )}
                </li>
              )
            })}
          </ol>
        </div>
      </div>
    </section>
  )
}
