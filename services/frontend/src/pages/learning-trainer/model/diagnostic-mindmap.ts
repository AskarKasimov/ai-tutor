import type { Edge, Node } from '@xyflow/react'
import type { CompetencyMapOutline } from '@/entities/learning-competency-map'

export type SiriusBranchColor = {
  name: string
  color: string
  lightBg: string
  border: string
  accent: string
  glow: string
}

export const SIRIUS_BRANCH_COLORS: SiriusBranchColor[] = [
  {
    name: 'indigo',
    color: '#4f46e5',
    lightBg: 'rgba(79, 70, 229, 0.08)',
    border: 'rgba(79, 70, 229, 0.35)',
    accent: '#6366f1',
    glow: 'rgba(99, 102, 241, 0.25)',
  },
  {
    name: 'teal',
    color: '#0d9488',
    lightBg: 'rgba(13, 148, 136, 0.08)',
    border: 'rgba(13, 148, 136, 0.35)',
    accent: '#14b8a6',
    glow: 'rgba(20, 184, 166, 0.25)',
  },
  {
    name: 'violet',
    color: '#7c3aed',
    lightBg: 'rgba(124, 58, 237, 0.08)',
    border: 'rgba(124, 58, 237, 0.35)',
    accent: '#8b5cf6',
    glow: 'rgba(139, 92, 246, 0.25)',
  },
  {
    name: 'amber',
    color: '#d97706',
    lightBg: 'rgba(217, 119, 6, 0.08)',
    border: 'rgba(217, 119, 6, 0.35)',
    accent: '#f59e0b',
    glow: 'rgba(245, 158, 11, 0.25)',
  },
  {
    name: 'rose',
    color: '#e11d48',
    lightBg: 'rgba(225, 29, 72, 0.08)',
    border: 'rgba(225, 29, 72, 0.35)',
    accent: '#f43f5e',
    glow: 'rgba(244, 63, 94, 0.25)',
  },
  {
    name: 'emerald',
    color: '#059669',
    lightBg: 'rgba(5, 150, 105, 0.08)',
    border: 'rgba(5, 150, 105, 0.35)',
    accent: '#10b981',
    glow: 'rgba(16, 185, 129, 0.25)',
  },
]

export type SubjectNodeData = {
  kind: 'subject'
  title: string
}

export type CompetencyNodeData = {
  kind: 'competency'
  id: string
  title: string
  cleanTitle: string
  side: 'left' | 'right'
  branchIndex: number
  color: string
  lightBg: string
  border: string
  accent: string
  glow: string
  constituentCount: number
}

export type ConstituentNodeData = {
  kind: 'constituent'
  id: string
  title: string
  cleanTitle: string
  competencyId: string
  competencyTitle: string
  side: 'left' | 'right'
  branchIndex: number
  color: string
  lightBg: string
  border: string
  accent: string
  glow: string
  outcomes: Array<{
    id: string
    name: string
    role?: 'main' | 'basic'
  }>
  isSelected: boolean
}

export type DiagnosticMindmapNode =
  | Node<SubjectNodeData, 'subject'>
  | Node<CompetencyNodeData, 'competency'>
  | Node<ConstituentNodeData, 'constituent'>

export type DiagnosticMindmap = {
  nodes: DiagnosticMindmapNode[]
  edges: Edge[]
}

export const SUBJECT_WIDTH = 220
export const SUBJECT_HEIGHT = 106
export const COMPETENCY_WIDTH = 200
export const COMPETENCY_HEIGHT = 94
export const CONSTITUENT_WIDTH = 200
export const CONSTITUENT_HEIGHT = 88
export const CONSTITUENT_GAP = 28
export const COMPETENCY_GAP = 42
export const HORIZ_COMPETENCY = 280
export const HORIZ_CONSTITUENT = 600

export function cleanCompetencyTitle(title: string): string {
  if (!title) return ''
  const cleaned = title
    // "Компетенция 1: ...", "Компетенция 1. ...", "Компетенция 1 ..."
    .replace(/^Компетенция\s*\d+(?:[.:\-—\s]+\s+|\s+)(?=\S)/i, '')
    // "К1: ...", "К1. ...", "К-1 ...", "К 1: ...", "К.1: ..."
    .replace(/^К\s*[-.]?\s*\d+(?:[.:\-—\s]+\s+|\s+)(?=\S)/i, '')
    // Leading numbers: "1. ...", "1: ...", "1.1. ..."
    .replace(/^\d+(?:\.\d+)*(?:[.:\-—\s]+\s+|\s+)(?=\S)/, '')
    .trim()
  return cleaned || title
}

export function cleanTopicTitle(title: string): string {
  if (!title) return ''
  const cleaned = title
    // "Тема 1. ", "Тема 1: ", "Тема 1 ", "Раздел 1. ", "§ 1. "
    .replace(/^(?:Тема|Раздел|§)\s*\d+(?:[.:\-—\s]+\s+|\s+)(?=\S)/i, '')
    // "1.1. ", "1.1 ", "1. ", "1) ", "1.1) "
    .replace(/^\d+(?:\.\d+)*(?:[.)\s:\-—]+\s+|\s+)(?=\S)/, '')
    .trim()
  return cleaned || title
}

function layoutSide(
  competencyItems: Array<{
    comp: CompetencyMapOutline['competencies'][number]
    branchIndex: number
  }>,
  side: 'left' | 'right',
  selectedConstituentId?: string | null,
): { nodes: DiagnosticMindmapNode[]; edges: Edge[] } {
  const nodes: DiagnosticMindmapNode[] = []
  const edges: Edge[] = []
  if (competencyItems.length === 0) return { nodes, edges }

  const competencyX =
    side === 'right' ? HORIZ_COMPETENCY : -HORIZ_COMPETENCY - COMPETENCY_WIDTH
  const constituentX =
    side === 'right'
      ? HORIZ_CONSTITUENT
      : -HORIZ_CONSTITUENT - CONSTITUENT_WIDTH

  const blockHeights = competencyItems.map(({ comp }) => {
    const m = comp.constituents.length
    return m > 0
      ? m * CONSTITUENT_HEIGHT + (m - 1) * CONSTITUENT_GAP
      : COMPETENCY_HEIGHT
  })

  const totalHeight =
    blockHeights.reduce((acc, h) => acc + h, 0) +
    (competencyItems.length - 1) * COMPETENCY_GAP

  let curY = -totalHeight / 2

  competencyItems.forEach(({ comp, branchIndex }, idx) => {
    const palette =
      SIRIUS_BRANCH_COLORS[branchIndex % SIRIUS_BRANCH_COLORS.length]
    const blockH = blockHeights[idx]

    const compY = curY + (blockH - COMPETENCY_HEIGHT) / 2
    const compNodeId = `competency:${comp.id}`
    const compClean = cleanCompetencyTitle(comp.name)

    nodes.push({
      id: compNodeId,
      type: 'competency',
      position: { x: competencyX, y: compY },
      data: {
        kind: 'competency',
        id: comp.id,
        title: comp.name,
        cleanTitle: compClean,
        side,
        branchIndex,
        color: palette.color,
        lightBg: palette.lightBg,
        border: palette.border,
        accent: palette.accent,
        glow: palette.glow,
        constituentCount: comp.constituents.length,
      },
    })

    const isCompActive = comp.constituents.some(
      (c) => c.id === selectedConstituentId,
    )

    edges.push({
      id: `edge-subject-${comp.id}`,
      source: 'subject-root',
      sourceHandle: side === 'right' ? 'source-right' : 'source-left',
      target: compNodeId,
      targetHandle: side === 'right' ? 'target-left' : 'target-right',
      type: 'bezier',
      animated: isCompActive,
      style: {
        stroke: palette.color,
        strokeWidth: isCompActive ? 3.5 : 2.5,
        opacity: isCompActive ? 1 : 0.8,
      },
    })

    comp.constituents.forEach((constituent, cIdx) => {
      const constY = curY + cIdx * (CONSTITUENT_HEIGHT + CONSTITUENT_GAP)
      const constNodeId = `constituent:${constituent.id}`
      const isSelected = selectedConstituentId === constituent.id
      const constClean = cleanTopicTitle(constituent.name)

      nodes.push({
        id: constNodeId,
        type: 'constituent',
        position: { x: constituentX, y: constY },
        data: {
          kind: 'constituent',
          id: constituent.id,
          title: constituent.name,
          cleanTitle: constClean,
          competencyId: comp.id,
          competencyTitle: comp.name,
          side,
          branchIndex,
          color: palette.color,
          lightBg: palette.lightBg,
          border: palette.border,
          accent: palette.accent,
          glow: palette.glow,
          outcomes: constituent.outcomes,
          isSelected,
        },
      })

      edges.push({
        id: `edge-${comp.id}-${constituent.id}`,
        source: compNodeId,
        sourceHandle: side === 'right' ? 'source-right' : 'source-left',
        target: constNodeId,
        targetHandle: side === 'right' ? 'target-left' : 'target-right',
        type: 'bezier',
        animated: isSelected,
        style: {
          stroke: palette.color,
          strokeWidth: isSelected ? 3.5 : 2,
          opacity: isSelected ? 1 : 0.75,
        },
      })
    })

    curY += blockH + COMPETENCY_GAP
  })

  return { nodes, edges }
}

export function layoutDiagnosticMindmap(
  outline: CompetencyMapOutline,
  subjectTitle: string,
  selectedConstituentId?: string | null,
): DiagnosticMindmap {
  const rootNode: DiagnosticMindmapNode = {
    id: 'subject-root',
    type: 'subject',
    position: { x: -SUBJECT_WIDTH / 2, y: -SUBJECT_HEIGHT / 2 },
    data: {
      kind: 'subject',
      title: subjectTitle,
    },
  }

  const competencies = outline.competencies
  if (competencies.length === 0) {
    return { nodes: [rootNode], edges: [] }
  }

  const rightItems: Array<{
    comp: CompetencyMapOutline['competencies'][number]
    branchIndex: number
  }> = []
  const leftItems: Array<{
    comp: CompetencyMapOutline['competencies'][number]
    branchIndex: number
  }> = []

  if (competencies.length === 1) {
    rightItems.push({ comp: competencies[0], branchIndex: 0 })
  } else {
    competencies.forEach((comp, i) => {
      if (i % 2 === 0) {
        rightItems.push({ comp, branchIndex: i })
      } else {
        leftItems.push({ comp, branchIndex: i })
      }
    })
  }

  const right = layoutSide(rightItems, 'right', selectedConstituentId)
  const left = layoutSide(leftItems, 'left', selectedConstituentId)

  return {
    nodes: [rootNode, ...right.nodes, ...left.nodes],
    edges: [...right.edges, ...left.edges],
  }
}
