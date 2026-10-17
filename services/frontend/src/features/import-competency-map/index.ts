export {
  useSubjectMaterialsQuery,
  useUploadCourseMaterialsMutation,
  useDeleteCourseMaterialMutation,
  useCourseGraphQuery,
  useBuildCourseGraphMutation,
  materialInProgress,
  useCourseMaterialsChanging,
  materialsRefetchInterval,
  courseGraphRefetchInterval,
} from './model/use-competency-map'
export {
  CompetencyMapDependenciesProvider,
  useCompetencyMapDependencies,
} from './model/dependencies-context'
export type { CompetencyMapDependencies } from './model/dependencies-context'
export {
  MaterialUploadQueue,
  MaterialUploadQueueToolbar,
  MaterialUploadSubmit,
  MaterialQueueCard,
  MaterialReorderControls,
} from './ui/material-upload-queue'
export { useMaterialUploadQueue } from './model/use-material-upload-queue'
export type { MaterialQueueItem } from './model/use-material-upload-queue'
