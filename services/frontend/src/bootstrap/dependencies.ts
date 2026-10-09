import { createMockTrainerSessionSource } from '@/bootstrap/demo-trainer-session-source'
import { mockApiFetch, evaluateDemoAnswer } from '@/bootstrap/mock-api'
import { configureMockApiHandler } from '@/shared/api'
import * as auth from '@/entities/user'
import * as competencyMap from '@/entities/competency-map'
import * as diagnostic from '@/entities/diagnostic-session'
import * as voice from '@/shared/api'
import { createDemoAudio as createDemoAudioBytes } from '@/shared/lib'
import * as assessment from '@/entities/assessment'
import type { AuthDependencies } from '@/features/auth'
import type { CompetencyMapDependencies } from '@/features/import-competency-map'
import type { DiagnosticDependencies } from '@/features/diagnostic-session'
import type { TrainerSessionDependencies } from '@/features/trainer-session'
import type { VoiceAnswerDependencies } from '@/features/voice-answer'
import { createDiagnosticIdentity } from '@/features/diagnostic-session'
import { mockDiagnosticStorage } from './mock-diagnostic-storage'
import {
  loadDiagnosticIdentity,
  saveDiagnosticIdentity,
} from '@/features/diagnostic-session'

export type AppDependencies = {
  mode: AuthDependencies['mode']
  auth: AuthDependencies['auth']
  competencyMap: CompetencyMapDependencies['competencyMap']
  apiBase: DiagnosticDependencies['apiBase']
  createDiagnosticIdentity: DiagnosticDependencies['createDiagnosticIdentity']
  diagnosticStorage: DiagnosticDependencies['diagnosticStorage']
  diagnostic: DiagnosticDependencies['diagnostic']
  trainerSessionSource: TrainerSessionDependencies['trainerSessionSource']
  voice: VoiceAnswerDependencies['voice']
  assessment: VoiceAnswerDependencies['assessment']
}

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)

export function createAppDependencies(): AppDependencies {
  const mode = import.meta.env.VITE_API_MODE === 'real' ? 'real' : 'demo'
  configureMockApiHandler(mockApiFetch)
  return {
    mode,
    apiBase,
    createDiagnosticIdentity,
    diagnosticStorage:
      mode === 'demo'
        ? mockDiagnosticStorage
        : { load: loadDiagnosticIdentity, save: saveDiagnosticIdentity },
    trainerSessionSource:
      mode === 'demo' ? createMockTrainerSessionSource() : undefined,
    auth: {
      readCurrentUser: auth.readCurrentUser,
      authenticate: auth.authenticate,
      logout: auth.logout,
    },
    competencyMap: {
      read: competencyMap.readCompetencyMap,
      import: competencyMap.importCompetencyMap,
    },
    diagnostic: {
      createVariant: diagnostic.createVariant,
      startDiagnostic: diagnostic.startDiagnostic,
      readDiagnostic: diagnostic.readDiagnostic,
      readDiagnosticResult: diagnostic.readDiagnosticResult,
      readDiagnosticFeedback: diagnostic.readDiagnosticFeedback,
      submitDiagnostic: diagnostic.submitDiagnostic,
      createSubmission: diagnostic.createSubmission,
      readDiagnosticAudio: diagnostic.readDiagnosticAudio,
      regenerateDiagnosticAudio: diagnostic.regenerateDiagnosticAudio,
      fetchDiagnosticAudioFile: diagnostic.fetchDiagnosticAudioFile,
    },
    voice: {
      transcribeRecording: voice.transcribeRecording,
      createDemoAudio: () =>
        new Blob([createDemoAudioBytes()], { type: 'audio/wav' }),
    },
    assessment: {
      evaluateDemoAnswer,
      fetchOverallFeedback: assessment.fetchOverallFeedback,
    },
  }
}
