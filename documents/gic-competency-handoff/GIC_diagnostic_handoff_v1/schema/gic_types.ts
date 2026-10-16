/** Shared API/data DTOs; place in a versioned contract package, not UI internals. */
export type TopicId = `AI-${string}`;
export type GateId = `DG-${string}`;
export type KnowledgeStatus =
  | 'unknown'
  | 'mastered_observed'
  | 'suspected_gap'
  | 'insufficient_evidence';
export type AssessmentOutcome = 'pass' | 'gap' | 'uncertain';
export type Grader = 'expert' | 'teacher' | 'rubric_engine';

export interface Topic {
  id: TopicId;
  title: string;
  section_id: number;
  section: string;
  kind: 'topic';
  material_status: 'available' | 'missing';
  status: 'draft_requires_review' | 'validated';
  is_atomic: boolean;
  is_terminal: boolean;
  question: string;
  provides: string[];
  requires: string[];
  rubric: string[];
  source_quality: string;
  source_pages: string[];
  full_text: string;
}

export interface Prerequisite {
  source: TopicId;
  target: TopicId;
  kind: 'prerequisite';
  requirement_ids: string[];
  requirements: string[];
  rationale: string[];
  validation_status: 'provisional' | 'validated' | 'rejected';
}

export interface DiagnosticGate {
  id: GateId;
  title: string;
  kind: 'diagnostic_group';
  assessment_mode: 'integrated_pilot' | 'routing_only';
  validation_status: 'draft_unvalidated' | 'validated';
  auto_close_allowed: boolean;
  terminal_topic_ids: TopicId[];
  closure_topic_ids: TopicId[];
  composite_prompt_draft: string;
  rubric_draft: Array<{topic_id: TopicId; criterion: string; status?: string}>;
  source_sections: number[];
  note: string;
}

export interface CourseGraph {
  schema_version: '1.0.0';
  dataset_id: string;
  provenance: Record<string, string>;
  semantics: Record<string, string>;
  topics: Topic[];
  prerequisites: Prerequisite[];
  diagnostic_groups: DiagnosticGate[];
  unmapped_requirements: Record<string, unknown>[];
  curriculum_gaps: Array<{id: string; title: string}>;
  external_review_status: string;
}

/** Client request for teacher/human or calibrated external scoring. */
export interface StartDiagnosisRequest {
  student_id: string;
  dataset_id: string;
  group_ids?: GateId[];
}
export interface QuestionDTO {
  question_id: string;
  topic_id: TopicId;
  group_id: GateId;
  prompt: string;
  rubric: string[];
  reason: 'terminal_screening' | 'prerequisite_drilldown';
  // There is no automatic whole-gate "pass" in the baseline.
}
export interface EvidenceDTO {
  evidence_id: string;
  topic_id: TopicId;
  group_id: GateId;
  outcome: AssessmentOutcome;
  answer: string;
  grader: Grader;
  rationale: string;
  created_at: string;
}
export interface SubmitEvidenceRequest {
  question_id: string;
  topic_id: TopicId;
  answer: string;
  outcome: AssessmentOutcome;
  grader: Grader;
  rationale: string;
}
export interface TopicProgressDTO {
  topic_id: TopicId;
  status: KnowledgeStatus;
  last_evidence_id?: string;
}
export interface LearningPlanEntry {
  topic_id: TopicId;
  title: string;
  level: number;
  status: KnowledgeStatus;
}
