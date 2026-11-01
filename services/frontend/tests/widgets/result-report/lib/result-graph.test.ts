import { expect, it } from 'vitest'
import { layoutResultGraph } from '@/widgets/result-report/lib/result-graph'
import type { ReportStatus, ReportTopic } from '@/widgets/result-report'

const topic = (
  id: string,
  requires: string[] = [],
  status: ReportStatus = 'full',
): ReportTopic => ({
  id,
  title: id,
  status,
  score: 1,
  maxScore: 1,
  assisted: false,
  skipped: false,
  requires,
  main: false,
  marks: [],
  note: '',
})

const columns = (graph: ReturnType<typeof layoutResultGraph>) =>
  Object.fromEntries(graph.nodes.map((n) => [n.topic.id, n.column]))

it('places prerequisites in columns left of the topics relying on them', () => {
  const graph = layoutResultGraph([
    topic('a-star', ['heuristic', 'search']),
    topic('heuristic', ['search']),
    topic('search'),
  ])
  expect(columns(graph)).toEqual({ search: 0, heuristic: 1, 'a-star': 2 })
  expect(graph.columns).toBe(3)
  // a-star → search is implied by a-star → heuristic → search.
  expect(graph.edges).toEqual([
    { from: 'heuristic', to: 'a-star', via: [] },
    { from: 'search', to: 'heuristic', via: [] },
  ])
})

it('hides unchecked topics but keeps the relations through them', () => {
  const graph = layoutResultGraph([
    topic('top', ['hidden']),
    topic('hidden', ['base'], 'unchecked'),
    topic('base'),
    topic('alone', [], 'unchecked'),
  ])
  expect(graph.nodes.map((n) => n.topic.id)).toEqual(['top', 'base'])
  expect(graph.edges).toEqual([{ from: 'base', to: 'top', via: [] }])
})

it('ignores prerequisites outside the result and survives a cycle', () => {
  const graph = layoutResultGraph([
    topic('a', ['b', 'missing']),
    topic('b', ['a']),
  ])
  expect(graph.nodes).toHaveLength(2)
  expect(graph.edges.every((e) => e.from !== 'missing')).toBe(true)
})

it('centres shorter columns and gives every node its own row', () => {
  const graph = layoutResultGraph([
    topic('top', ['x', 'y', 'z']),
    topic('x'),
    topic('y'),
    topic('z'),
  ])
  expect(graph.rows).toBe(3)
  const top = graph.nodes.find((n) => n.topic.id === 'top')!
  expect(top.row).toBe(1)
  const base = graph.nodes.filter((n) => n.column === 0).map((n) => n.row)
  expect(new Set(base).size).toBe(3)
})

it('returns an empty map without checked topics', () => {
  const graph = layoutResultGraph([topic('a', [], 'unchecked')])
  expect(graph.nodes).toEqual([])
  expect(graph.edges).toEqual([])
})

it('gives a long edge its own lane slot in every column it crosses', () => {
  const graph = layoutResultGraph([
    topic('top', ['middle', 'side']),
    topic('middle', ['base']),
    topic('base'),
    topic('side'),
    topic('deep', ['base']),
    topic('deeper', ['deep']),
    topic('end', ['deeper', 'side']),
  ])
  const edge = graph.edges.find((e) => e.from === 'side' && e.to === 'end')!
  expect(edge.via.map((v) => v.column)).toEqual([1, 2])
  // The lane never shares a slot with a card.
  for (const v of edge.via)
    expect(
      graph.nodes.some((n) => n.column === v.column && n.row === v.row),
    ).toBe(false)
})
