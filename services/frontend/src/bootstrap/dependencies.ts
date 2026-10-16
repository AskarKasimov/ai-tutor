import * as auth from '@/entities/user'
import * as competencyMap from '@/entities/competency-map'
import * as subjects from '@/entities/subject'
import * as diagnostic from '@/entities/diagnostic-session'
import type { AuthDependencies } from '@/features/auth'
import type { CompetencyMapDependencies } from '@/features/import-competency-map'
import type { DiagnosticDependencies } from '@/features/diagnostic-session'
import type { SubjectSelectionDependencies } from '@/features/subject-selection'
import { transcribeRecording } from '@/shared/api'

export type AppDependencies = {
  auth: AuthDependencies['auth']
  competencyMap: CompetencyMapDependencies['competencyMap']
  subjects: SubjectSelectionDependencies['subjects']
  diagnostic: DiagnosticDependencies['diagnostic']
}

export function createAppDependencies(): AppDependencies {
  return {
    auth: {
      readCurrentUser: auth.readCurrentUser,
      authenticate: auth.authenticate,
      logout: auth.logout,
    },
    competencyMap: {
      readSubjectMaterials: competencyMap.readSubjectMaterials,
      uploadCourseMaterials: competencyMap.uploadCourseMaterials,
      deleteCourseMaterial: competencyMap.deleteCourseMaterial,
    },
    subjects: {
      listSubjects: subjects.listSubjects,
      createSubject: subjects.createSubject,
    },
    diagnostic: {
      transcribeRecording,
      startDiagnostic: diagnostic.startDiagnostic,
      readDiagnostic: diagnostic.readDiagnostic,
      listDiagnostics: diagnostic.listDiagnostics,
      answerDiagnostic: diagnostic.answerDiagnostic,
      readDiagnosticResult: diagnostic.readDiagnosticResult,
      fetchTurnAudio: diagnostic.fetchTurnAudio,
    },
  }
}
