import { createMockTrainerSessionSource } from '@/bootstrap/demo-trainer-session-source'
import { mockApiFetch, evaluateDemoAnswer } from '@/bootstrap/mock-api'
import { configureMockApiHandler } from '@/shared/api'
import * as auth from '@/entities/user'
import * as competencyMap from '@/entities/competency-map'
import * as subjects from '@/entities/subject'
import * as diagnostic from '@/entities/diagnostic-session'
import * as voice from '@/shared/api'
import { createDemoAudio as createDemoAudioBytes } from '@/shared/lib'
import * as assessment from '@/entities/assessment'
import type { AuthDependencies } from '@/features/auth'
import type { CompetencyMapDependencies } from '@/features/import-competency-map'
import type { DiagnosticDependencies } from '@/features/diagnostic-session'
import type { TrainerSessionDependencies } from '@/features/trainer-session'
import type { SubjectSelectionDependencies } from '@/features/subject-selection'
import type { VoiceAnswerDependencies } from '@/features/voice-answer'
import type { TrainingDependencies } from '@/features/training'
import * as training from '@/entities/training'
import { createDiagnosticIdentity } from '@/features/diagnostic-session'
import { mockDiagnosticStorage } from './mock-diagnostic-storage'
import { diagnosticSessionStorage } from '@/features/diagnostic-session'

export type AppDependencies = {
  mode: AuthDependencies['mode']
  auth: AuthDependencies['auth']
  competencyMap: CompetencyMapDependencies['competencyMap']
  subjects: SubjectSelectionDependencies['subjects']
  apiBase: DiagnosticDependencies['apiBase']
  createDiagnosticIdentity: DiagnosticDependencies['createDiagnosticIdentity']
  diagnosticStorage: DiagnosticDependencies['diagnosticStorage']
  diagnostic: DiagnosticDependencies['diagnostic']
  trainerSessionSource: TrainerSessionDependencies['trainerSessionSource']
  voice: VoiceAnswerDependencies['voice']
  assessment: VoiceAnswerDependencies['assessment']
  training: TrainingDependencies['training']
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
      mode === 'demo' ? mockDiagnosticStorage : diagnosticSessionStorage,
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
    subjects: {
      listSubjects: subjects.listSubjects,
      createSubject: subjects.createSubject,
      readLearningState: subjects.readLearningState,
    },
    diagnostic: {
      createVariant: diagnostic.createVariant,
      startDiagnostic: diagnostic.startDiagnostic,
      readDiagnostic: diagnostic.readDiagnostic,
      readDiagnosticResult: diagnostic.readDiagnosticResult,
      readDiagnosticFeedback: diagnostic.readDiagnosticFeedback,
      submitDiagnostic: diagnostic.submitDiagnostic,
      createSubmission: diagnostic.createSubmission,
      createSkipSubmission: diagnostic.createSkipSubmission,
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
    training: {
      readTrainingPreview: training.readTrainingPreview,
      startTraining: training.startTraining,
      findTrainingForDiagnostic: training.findTrainingForDiagnostic,
      readTrainingSession: training.readTrainingSession,
      createTrainingSubmission: training.createTrainingSubmission,
      createTrainingSkipSubmission: training.createTrainingSkipSubmission,
      submitTraining: training.submitTraining,
      resetTrainingAnswer: training.resetTrainingAnswer,
      readTrainingHistory: training.readTrainingHistory,
      readTrainingAudio: training.readTrainingAudio,
      fetchTrainingAudioFile: training.fetchTrainingAudioFile,
      regenerateTrainingAudio: training.regenerateTrainingAudio,
    },
  }
}
