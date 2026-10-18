import type { TopicResult } from '@/entities/diagnostic-session'

export type GraphNode = {
  topic: TopicResult
  column: number
  row: number
}

export type GraphEdge = {
  // The prerequisite and the topic that relies on it.
  from: string
  to: string
  // Slots the edge passes in the columns between them, left to right, so a
  // long edge runs in its own lane instead of under other cards.
  via: { column: number; row: number }[]
}

export type ResultGraph = {
  nodes: GraphNode[]
  edges: GraphEdge[]
  columns: number
  rows: number
}

// The subgraph of the topics the diagnostic checked, laid out in columns from
// the basic topics on the left to the topics relying on them on the right.
export function layoutResultGraph(topics: TopicResult[]): ResultGraph {
  const byId = new Map(topics.map((topic) => [topic.nodeId, topic]))
  const shown = topics.filter((topic) => topic.status !== 'unchecked')
  const visible = new Set(shown.map((topic) => topic.nodeId))

  // A hidden topic between two shown ones keeps their relation: walk through
  // hidden prerequisites to the nearest shown ones.
  const requires = new Map<string, string[]>()
  for (const topic of shown) {
    const found = new Set<string>()
    const seen = new Set<string>()
    const stack = [...topic.requires]
    while (stack.length > 0) {
      const id = stack.pop()!
      if (seen.has(id) || id === topic.nodeId) continue
      seen.add(id)
      if (visible.has(id)) found.add(id)
      else stack.push(...(byId.get(id)?.requires ?? []))
    }
    requires.set(topic.nodeId, [...found])
  }
  // Keep only direct steps: an edge already implied by a longer path is noise.
  for (const [id, list] of requires) {
    requires.set(
      id,
      list.filter(
        (p) =>
          !list.some((other) => other !== p && reaches(requires, other, p)),
      ),
    )
  }

  // The column of a topic is one more than its deepest prerequisite.
  const column = new Map<string, number>()
  const place = (id: string, path: Set<string>): number => {
    const known = column.get(id)
    if (known !== undefined) return known
    // A cycle never comes from a valid graph; break it instead of looping.
    if (path.has(id)) return 0
    path.add(id)
    const deps = requires.get(id) ?? []
    const value =
      deps.length === 0 ? 0 : 1 + Math.max(...deps.map((d) => place(d, path)))
    path.delete(id)
    column.set(id, value)
    return value
  }
  shown.forEach((topic) => place(topic.nodeId, new Set()))
  const columns = Math.max(0, ...column.values()) + 1

  // An edge longer than one column gets a lane slot in every column it
  // crosses; slots are ordered together with the cards.
  const left = new Map<string, string[]>()
  const right = new Map<string, string[]>()
  const link = (from: string, to: string) => {
    left.set(to, [...(left.get(to) ?? []), from])
    right.set(from, [...(right.get(from) ?? []), to])
  }
  const layers: string[][] = Array.from({ length: columns }, () => [])
  shown.forEach((topic) => layers[column.get(topic.nodeId)!].push(topic.nodeId))
  const chains: { from: string; to: string; lanes: string[] }[] = []
  for (const [to, list] of requires) {
    for (const from of list) {
      const lanes: string[] = []
      let previous = from
      for (let c = column.get(from)! + 1; c < column.get(to)!; c++) {
        const lane = `${from}\u0000${to}\u0000${c}`
        layers[c].push(lane)
        lanes.push(lane)
        link(previous, lane)
        previous = lane
      }
      link(previous, to)
      chains.push({ from, to, lanes })
    }
  }

  // Rows: keep the source order, then pull each slot towards the average row
  // of its neighbours a few times to straighten the edges.
  const row = new Map<string, number>()
  const index = () =>
    layers.forEach((layer) => layer.forEach((id, i) => row.set(id, i)))
  index()
  const mean = (ids: string[], fallback: number) =>
    ids.length === 0
      ? fallback
      : ids.reduce((sum, id) => sum + row.get(id)!, 0) / ids.length
  for (let pass = 0; pass < 6; pass++) {
    const forward = pass % 2 === 0
    const order = forward ? layers : [...layers].reverse()
    for (const layer of order) {
      const weight = new Map(
        layer.map((id) => [
          id,
          mean((forward ? left.get(id) : right.get(id)) ?? [], row.get(id)!),
        ]),
      )
      layer.sort(
        (a, b) => weight.get(a)! - weight.get(b)! || row.get(a)! - row.get(b)!,
      )
      index()
    }
  }

  const rows = Math.max(0, ...layers.map((layer) => layer.length))
  // Shorter columns sit in the middle of the tallest one.
  const slot = (id: string, c: number) =>
    row.get(id)! + (rows - layers[c].length) / 2
  return {
    nodes: shown.map((topic) => ({
      topic,
      column: column.get(topic.nodeId)!,
      row: slot(topic.nodeId, column.get(topic.nodeId)!),
    })),
    edges: chains.map(({ from, to, lanes }) => ({
      from,
      to,
      via: lanes.map((lane, i) => {
        const c = column.get(from)! + 1 + i
        return { column: c, row: slot(lane, c) }
      }),
    })),
    columns,
    rows,
  }
}

function reaches(requires: Map<string, string[]>, from: string, to: string) {
  const seen = new Set<string>()
  const stack = [...(requires.get(from) ?? [])]
  while (stack.length > 0) {
    const id = stack.pop()!
    if (id === to) return true
    if (seen.has(id)) continue
    seen.add(id)
    stack.push(...(requires.get(id) ?? []))
  }
  return false
}
