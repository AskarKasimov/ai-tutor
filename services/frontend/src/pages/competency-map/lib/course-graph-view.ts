import type { CourseGraphView as CourseGraph } from '@/entities/course-graph'

type CourseGraphTopic = CourseGraph['topics'][number]

// Tones are Radix color scales; a section keeps its tone in nodes, edges,
// the minimap and the legend.
export const SECTION_TONES = [
  'blue',
  'green',
  'violet',
  'orange',
  'cyan',
  'crimson',
  'amber',
  'teal',
  'indigo',
  'pink',
  'grass',
  'brown',
] as const
export type SectionTone = (typeof SECTION_TONES)[number] | 'gray'

export type GraphSection = {
  // The id of the top-level topic, or null for topics outside the hierarchy.
  id: string | null
  title: string | null
  tone: SectionTone
  size: number
}

export type GraphLink = { id: string; reason?: string; requirement?: string }

export type GraphTopicView = {
  topic: CourseGraphTopic
  section: GraphSection
  // A top-level topic that groups others.
  isSection: boolean
  parentId: string | null
  childIds: string[]
  // Topics this one builds on (incoming prerequisites).
  prerequisites: GraphLink[]
  // Topics that build on this one (outgoing prerequisites).
  dependents: GraphLink[]
}

export type CourseGraphView = {
  topics: Map<string, GraphTopicView>
  // Topics in graph order.
  order: string[]
  sections: GraphSection[]
  prerequisiteEdges: { id: string; source: string; target: string }[]
  hierarchyEdges: { id: string; source: string; target: string }[]
}

export function buildCourseGraphView(graph: CourseGraph): CourseGraphView {
  const known = new Set(graph.topics.map((topic) => topic.id))
  const parentOf = new Map<string, string>()
  const childrenOf = new Map<string, string[]>()
  const hierarchyEdges: CourseGraphView['hierarchyEdges'] = []
  for (const edge of graph.hierarchy) {
    if (!known.has(edge.source) || !known.has(edge.target)) continue
    if (edge.source === edge.target || parentOf.has(edge.target)) continue
    parentOf.set(edge.target, edge.source)
    childrenOf.set(edge.source, [
      ...(childrenOf.get(edge.source) ?? []),
      edge.target,
    ])
    hierarchyEdges.push({
      id: `h:${edge.source}->${edge.target}`,
      source: edge.source,
      target: edge.target,
    })
  }

  // The service keeps the hierarchy acyclic; the visited set only guards the walk.
  const rootOf = (id: string) => {
    const visited = new Set([id])
    let current = id
    for (let parent = parentOf.get(current); parent;) {
      if (visited.has(parent)) break
      visited.add(parent)
      current = parent
      parent = parentOf.get(current)
    }
    return current
  }

  const sectionsById = new Map<string, GraphSection>()
  const loose: GraphSection = { id: null, title: null, tone: 'gray', size: 0 }
  const sectionOf = new Map<string, GraphSection>()
  for (const topic of graph.topics) {
    const root = rootOf(topic.id)
    if (root === topic.id && !childrenOf.has(topic.id)) {
      loose.size += 1
      sectionOf.set(topic.id, loose)
      continue
    }
    let section = sectionsById.get(root)
    if (!section) {
      section = {
        id: root,
        title: graph.topics.find((t) => t.id === root)?.title ?? root,
        tone: SECTION_TONES[sectionsById.size % SECTION_TONES.length],
        size: 0,
      }
      sectionsById.set(root, section)
    }
    section.size += 1
    sectionOf.set(topic.id, section)
  }

  const prerequisitesOf = new Map<string, GraphLink[]>()
  const dependentsOf = new Map<string, GraphLink[]>()
  const prerequisiteEdges: CourseGraphView['prerequisiteEdges'] = []
  const seen = new Set<string>()
  for (const edge of graph.prerequisites) {
    if (!known.has(edge.source) || !known.has(edge.target)) continue
    if (edge.source === edge.target) continue
    // One requirement can be met by several links between the same pair;
    // the picture draws one arrow and the details list every reason.
    const id = `p:${edge.source}->${edge.target}`
    const link = { reason: edge.reason, requirement: edge.requirement }
    prerequisitesOf.set(edge.target, [
      ...(prerequisitesOf.get(edge.target) ?? []),
      { id: edge.source, ...link },
    ])
    dependentsOf.set(edge.source, [
      ...(dependentsOf.get(edge.source) ?? []),
      { id: edge.target, ...link },
    ])
    if (seen.has(id)) continue
    seen.add(id)
    prerequisiteEdges.push({ id, source: edge.source, target: edge.target })
  }

  const topics = new Map<string, GraphTopicView>()
  for (const topic of graph.topics) {
    topics.set(topic.id, {
      topic,
      section: sectionOf.get(topic.id) ?? loose,
      isSection: !parentOf.has(topic.id) && childrenOf.has(topic.id),
      parentId: parentOf.get(topic.id) ?? null,
      childIds: childrenOf.get(topic.id) ?? [],
      prerequisites: prerequisitesOf.get(topic.id) ?? [],
      dependents: dependentsOf.get(topic.id) ?? [],
    })
  }

  const sections = [...sectionsById.values()]
  if (loose.size > 0) sections.push(loose)
  return {
    topics,
    order: graph.topics.map((topic) => topic.id),
    sections,
    prerequisiteEdges,
    hierarchyEdges,
  }
}

export const NODE_WIDTH = 248

// Card height grows with the title; the canvas measures the real size, the
// layout only needs a close guess to keep rows apart.
export function estimateNodeHeight(view: GraphTopicView) {
  const charsPerLine = view.isSection ? 24 : 28
  const lines = Math.min(3, Math.ceil(view.topic.title.length / charsPerLine))
  return 52 + Math.max(1, lines) * 20
}

export type NodePosition = { x: number; y: number }

export type GraphCluster = {
  section: GraphSection
  x: number
  y: number
  width: number
  height: number
}

export type CourseGraphLayout = {
  // Top-left corners in canvas coordinates.
  positions: Map<string, NodePosition>
  clusters: GraphCluster[]
}

const CLUSTER_PADDING = 20
const CLUSTER_HEADER = 36
const RANK_GAP = 72
const WRAP_GAP = 24
const ROW_GAP = 20
const CLUSTER_GAP = 56
// Width to height of the packed picture, close to a wide canvas.
const TARGET_ASPECT = 1.6

// Each section is an island: inside it learning order runs left to right
// (a parent or prerequisite stays left of the topics building on it), and a
// crowded column wraps into several so an island stays compact. Islands keep
// the course order and are packed into rows close to the canvas proportions.
export function layoutCourseGraph(view: CourseGraphView): CourseGraphLayout {
  const positions = new Map<string, NodePosition>()
  const islands = view.sections.map((section) =>
    layoutIsland(
      view,
      section,
      view.order.filter((id) => view.topics.get(id)!.section === section),
    ),
  )

  const area = islands.reduce((sum, i) => sum + i.width * i.height, 0)
  const rowWidth = Math.max(
    ...islands.map((island) => island.width),
    Math.sqrt(area * TARGET_ASPECT),
  )
  const clusters: GraphCluster[] = []
  let x = 0
  let y = 0
  let rowHeight = 0
  islands.forEach((island, index) => {
    if (x > 0 && x + island.width > rowWidth) {
      x = 0
      y += rowHeight + CLUSTER_GAP
      rowHeight = 0
    }
    clusters.push({
      section: view.sections[index],
      x,
      y,
      width: island.width,
      height: island.height,
    })
    for (const [id, local] of island.positions)
      positions.set(id, { x: x + local.x, y: y + local.y })
    x += island.width + CLUSTER_GAP
    rowHeight = Math.max(rowHeight, island.height)
  })
  return { positions, clusters }
}

function layoutIsland(
  view: CourseGraphView,
  section: GraphSection,
  members: string[],
) {
  const inside = new Set(members)
  const incoming = new Map<string, string[]>()
  const outgoing = new Map<string, string[]>()
  const link = (source: string, target: string) => {
    incoming.set(target, [...(incoming.get(target) ?? []), source])
    outgoing.set(source, [...(outgoing.get(source) ?? []), target])
  }
  const reaches = (from: string, to: string) => {
    const seen = new Set<string>()
    const stack = [from]
    while (stack.length > 0) {
      const id = stack.pop()!
      if (id === to) return true
      if (seen.has(id)) continue
      seen.add(id)
      stack.push(...(outgoing.get(id) ?? []))
    }
    return false
  }
  // The order of study follows the prerequisites. The service keeps them and
  // the hierarchy acyclic apart, not together, so a parent is put before its
  // subtopic only where that does not contradict a prerequisite.
  for (const edge of view.prerequisiteEdges)
    if (inside.has(edge.source) && inside.has(edge.target))
      link(edge.source, edge.target)
  for (const edge of view.hierarchyEdges)
    if (
      inside.has(edge.source) &&
      inside.has(edge.target) &&
      !reaches(edge.target, edge.source)
    )
      link(edge.source, edge.target)

  // Longest path from the island's sources. Should the data still hold a
  // cycle, its first topic is released so the topics after it get ranks too.
  const rank = new Map<string, number>()
  const pending = new Map(
    members.map((id) => [id, incoming.get(id)?.length ?? 0]),
  )
  const queue = members.filter((id) => pending.get(id) === 0)
  const done = new Set<string>()
  for (const id of members) rank.set(id, 0)
  while (done.size < members.length) {
    if (queue.length === 0) queue.push(members.find((id) => !done.has(id))!)
    const id = queue.shift()!
    if (done.has(id)) continue
    done.add(id)
    for (const next of outgoing.get(id) ?? []) {
      if (done.has(next)) continue
      rank.set(next, Math.max(rank.get(next)!, rank.get(id)! + 1))
      pending.set(next, pending.get(next)! - 1)
      if (pending.get(next) === 0) queue.push(next)
    }
  }

  const columns: string[][] = []
  for (const id of members) (columns[rank.get(id)!] ??= []).push(id)
  const maxRows = Math.min(7, Math.max(3, Math.ceil(Math.sqrt(members.length))))

  // Rows follow the neighbours on the left so arrows cross less.
  const row = new Map<string, number>()
  const positions = new Map<string, NodePosition>()
  let left = CLUSTER_PADDING
  let bottom = 0
  for (const column of columns.filter(Boolean)) {
    const order = column
      .map((id, index) => {
        const sources = (incoming.get(id) ?? []).filter((s) => row.has(s))
        const weight =
          sources.length > 0
            ? sources.reduce((sum, s) => sum + row.get(s)!, 0) / sources.length
            : Number.MAX_SAFE_INTEGER
        return { id, weight, index }
      })
      .sort((a, b) => a.weight - b.weight || a.index - b.index)
      .map((item) => item.id)
    order.forEach((id, index) => row.set(id, index))

    for (let start = 0; start < order.length; start += maxRows) {
      let top = CLUSTER_HEADER + CLUSTER_PADDING
      for (const id of order.slice(start, start + maxRows)) {
        positions.set(id, { x: left, y: top })
        top += estimateNodeHeight(view.topics.get(id)!) + ROW_GAP
      }
      bottom = Math.max(bottom, top - ROW_GAP)
      left += NODE_WIDTH + WRAP_GAP
    }
    left += RANK_GAP - WRAP_GAP
  }
  return {
    section,
    positions,
    width: left - RANK_GAP + CLUSTER_PADDING,
    height: bottom + CLUSTER_PADDING,
  }
}

// Ids of the topic itself and every topic it is directly linked to.
export function neighbourhood(view: CourseGraphView, id: string) {
  const topic = view.topics.get(id)
  const ids = new Set<string>([id])
  if (!topic) return ids
  for (const link of topic.prerequisites) ids.add(link.id)
  for (const link of topic.dependents) ids.add(link.id)
  if (topic.parentId) ids.add(topic.parentId)
  for (const child of topic.childIds) ids.add(child)
  return ids
}

export function searchTopics(view: CourseGraphView, query: string) {
  const needle = query.trim().toLocaleLowerCase('ru')
  if (!needle) return []
  return view.order.filter((id) =>
    view.topics.get(id)!.topic.title.toLocaleLowerCase('ru').includes(needle),
  )
}
