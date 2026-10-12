import * as auth from '@/entities/user'
import * as competencyMap from '@/entities/competency-map'
import * as subjects from '@/entities/subject'
import * as diagnostic from '@/entities/diagnostic-session'
import type { AuthDependencies } from '@/features/auth'
import type { CompetencyMapDependencies } from '@/features/import-competency-map'
import type { DiagnosticDependencies } from '@/features/diagnostic-session'
import type { SubjectSelectionDependencies } from '@/features/subject-selection'
import type { TrainingDependencies } from '@/features/training'
import * as training from '@/entities/training'
import { createDiagnosticIdentity } from '@/features/diagnostic-session'
import { diagnosticSessionStorage } from '@/features/diagnostic-session'

export type AppDependencies = {
  auth: AuthDependencies['auth']
  competencyMap: CompetencyMapDependencies['competencyMap']
  subjects: SubjectSelectionDependencies['subjects']
  apiBase: DiagnosticDependencies['apiBase']
  createDiagnosticIdentity: DiagnosticDependencies['createDiagnosticIdentity']
  diagnosticStorage: DiagnosticDependencies['diagnosticStorage']
  diagnostic: DiagnosticDependencies['diagnostic']
  training: TrainingDependencies['training']
}

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)

export function createAppDependencies(): AppDependencies {
  return {
    apiBase,
    createDiagnosticIdentity,
    diagnosticStorage: diagnosticSessionStorage,
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
    training: {
      readTrainingPreview: training.readTrainingPreview,
      startTraining: training.startTraining,
      findTrainingForDiagnostic: training.findTrainingForDiagnostic,
      readTrainingSession: training.readTrainingSession,
      createTrainingSubmission: training.createTrainingSubmission,
      createTrainingSkipSubmission: training.createTrainingSkipSubmission,
      submitTraining: training.submitTraining,
      nextTrainingExercise: training.nextTrainingExercise,
      resetTrainingAnswer: training.resetTrainingAnswer,
      readTrainingHistory: training.readTrainingHistory,
      readTrainingAudio: training.readTrainingAudio,
      fetchTrainingAudioFile: training.fetchTrainingAudioFile,
      regenerateTrainingAudio: training.regenerateTrainingAudio,
      readTrainingChat: training.readTrainingChat,
      createTrainingChatText: training.createTrainingChatText,
      createTrainingChatVoice: training.createTrainingChatVoice,
      sendTrainingChatText: training.sendTrainingChatText,
      sendTrainingChatVoice: training.sendTrainingChatVoice,
      retryTrainingChatTurn: training.retryTrainingChatTurn,
      cancelTrainingChatTurn: training.cancelTrainingChatTurn,
    },
  }
}
