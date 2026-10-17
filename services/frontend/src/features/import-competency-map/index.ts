export {
  useSubjectMaterialsQuery,
  useUploadCourseMaterialsMutation,
  useReplaceCourseMaterialMutation,
  useUpdateCourseMaterialMetadataMutation,
  useDeleteCourseMaterialMutation,
  useCourseGraphQuery,
  useBuildCourseGraphMutation,
  materialInProgress,
  materialsRefetchInterval,
  courseGraphRefetchInterval,
} from './model/use-competency-map'
export {
  CompetencyMapDependenciesProvider,
  useCompetencyMapDependencies,
} from './model/dependencies-context'
export type { CompetencyMapDependencies } from './model/dependencies-context'

export { MaterialUploadQueue } from './ui/material-upload-queue'
