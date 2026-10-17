import { expect, it } from 'vitest'
import type { CourseGraph } from '@/entities/competency-map'
import {
  NODE_WIDTH,
  buildCourseGraphView,
  estimateNodeHeight,
  layoutCourseGraph,
  neighbourhood,
  searchTopics,
} from '@/pages/competency-map/lib/course-graph-view'

const topic = (id: string, title: string) => ({ id, title })

const graph: CourseGraph = {
  topics: [
    topic('ml', 'Машинное обучение'),
    topic('loss', 'Функция потерь'),
    topic('gd', 'Градиентный спуск'),
    topic('stats', 'Статистика'),
    topic('mean', 'Среднее значение'),
    topic('alone', 'Отдельная тема'),
  ],
  hierarchy: [
    { source: 'ml', target: 'loss' },
    { source: 'ml', target: 'gd' },
    { source: 'stats', target: 'mean' },
    // A second parent and an unknown topic are ignored.
    { source: 'stats', target: 'gd' },
    { source: 'ml', target: 'missing' },
  ],
  prerequisites: [
    { source: 'loss', target: 'gd', reason: 'Спуск минимизирует потери' },
    { source: 'loss', target: 'gd', reason: 'Нужна производная потерь' },
    { source: 'mean', target: 'loss' },
  ],
}

it('groups topics into sections by their top-level parent', () => {
  const view = buildCourseGraphView(graph)
  expect(view.sections.map((s) => [s.title, s.size])).toEqual([
    ['Машинное обучение', 3],
    ['Статистика', 2],
    [null, 1],
  ])
  expect(view.topics.get('gd')!.section.title).toBe('Машинное обучение')
  expect(view.topics.get('gd')!.parentId).toBe('ml')
  expect(view.topics.get('ml')!.isSection).toBe(true)
  expect(view.topics.get('alone')!.section.tone).toBe('gray')
  expect(view.hierarchyEdges).toHaveLength(3)
})

it('draws one arrow per pair and keeps every reason for the details', () => {
  const view = buildCourseGraphView(graph)
  expect(view.prerequisiteEdges.map((e) => e.id)).toEqual([
    'p:loss->gd',
    'p:mean->loss',
  ])
  expect(view.topics.get('gd')!.prerequisites.map((l) => l.reason)).toEqual([
    'Спуск минимизирует потери',
    'Нужна производная потерь',
  ])
  expect(view.topics.get('loss')!.dependents.map((l) => l.id)).toEqual([
    'gd',
    'gd',
  ])
  expect([...neighbourhood(view, 'loss')].sort()).toEqual(
    ['gd', 'loss', 'mean', 'ml'].sort(),
  )
})

it('places prerequisites left of their topics without overlapping cards', () => {
  const view = buildCourseGraphView(graph)
  const { positions, clusters } = layoutCourseGraph(view)
  expect(positions.size).toBe(graph.topics.length)
  expect(clusters).toHaveLength(3)
  expect(positions.get('ml')!.x).toBeLessThan(positions.get('loss')!.x)
  expect(positions.get('loss')!.x).toBeLessThan(positions.get('gd')!.x)

  const boxes = view.order.map((id) => ({
    ...positions.get(id)!,
    height: estimateNodeHeight(view.topics.get(id)!),
  }))
  for (const [i, a] of boxes.entries())
    for (const b of boxes.slice(i + 1))
      expect(
        a.x + NODE_WIDTH <= b.x ||
          b.x + NODE_WIDTH <= a.x ||
          a.y + a.height <= b.y ||
          b.y + b.height <= a.y,
      ).toBe(true)

  // Every card stays inside its own island.
  for (const id of view.order) {
    const cluster = clusters.find(
      (c) => c.section === view.topics.get(id)!.section,
    )!
    const { x, y } = positions.get(id)!
    expect(x).toBeGreaterThanOrEqual(cluster.x)
    expect(x + NODE_WIDTH).toBeLessThanOrEqual(cluster.x + cluster.width)
    expect(y).toBeGreaterThanOrEqual(cluster.y)
  }
})

it('wraps a crowded column so a large section stays compact', () => {
  const children = Array.from({ length: 20 }, (_, i) =>
    topic(`t${i}`, `Тема ${i}`),
  )
  const view = buildCourseGraphView({
    topics: [topic('root', 'Раздел'), ...children],
    hierarchy: children.map((c) => ({ source: 'root', target: c.id })),
    prerequisites: [],
  })
  const { clusters } = layoutCourseGraph(view)
  expect(clusters[0].height).toBeLessThan(clusters[0].width)
})

it('finds topics by a case-insensitive part of the title', () => {
  const view = buildCourseGraphView(graph)
  expect(searchTopics(view, 'ПОТЕР')).toEqual(['loss'])
  expect(searchTopics(view, '  ')).toEqual([])
})
