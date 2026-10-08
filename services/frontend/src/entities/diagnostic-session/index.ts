export type {
  DiagnosticAnswer,
  DiagnosticAudioMetadata,
  DiagnosticProgress,
  DiagnosticResult,
  DiagnosticTask,
} from './model/diagnostic-session'
export type { DiagnosticSessionIdentity } from './model/session-identity'
export type { DiagnosticSubmission } from './model/submission'
export { diagnosticApiBase } from './api/diagnostic-api'
export { DiagnosticApiError } from './model/diagnostic-error'
export {
  createSubmission,
  createVariant,
  fetchDiagnosticAudioFile,
  readDiagnosticAudio,
  readDiagnostic,
  readDiagnosticResult,
  startDiagnostic,
  submitDiagnostic,
} from './api/diagnostic-api'
