export type {
  AnswerSource,
  DiagnosticAnswer,
  DiagnosticResult,
  DiagnosticSession,
  DiagnosticSummary,
  MaterialReference,
  SourceCitation,
  LearningMaterials,
  DiagnosticGap,
  Grade,
  SessionStatus,
  Speaker,
  TopicResult,
  TopicStatus,
  Turn,
  TurnAudioPart,
  TurnKind,
} from './model/diagnostic-session'
export { DiagnosticApiError } from './model/diagnostic-error'
export {
  answerDiagnostic,
  fetchTurnAudio,
  listDiagnostics,
  readDiagnostic,
  readDiagnosticResult,
  fetchDiagnosticMaterialRaw,
  startDiagnostic,
} from './api/diagnostic-api'
export type { DiagnosticApi } from './api/diagnostic-api'
