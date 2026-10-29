import type { CourseGraphView } from '@/entities/course-graph'
import type { CompetencyMapOutline } from '@/entities/learning-competency-map'

// The map defines containment, not prerequisites. Draw only those real links.
// A constituent may contain both main and basic outcomes and appears just once.
export function buildDiagnosticGraph(
  outline: CompetencyMapOutline,
): CourseGraphView {
  const graph: CourseGraphView = {
    topics: [],
    hierarchy: [],
    prerequisites: [],
  }
  for (const competency of outline.competencies) {
    const rootId = `competency:${competency.id}`
    graph.topics.push({ id: rootId, title: competency.name })
    for (const topic of competency.constituents) {
      const id = `constituent:${topic.id}`
      graph.topics.push({
        id,
        title: topic.name,
        provides: topic.outcomes.map((outcome) => outcome.name),
      })
      graph.hierarchy.push({ source: rootId, target: id })
    }
  }
  return graph
}
