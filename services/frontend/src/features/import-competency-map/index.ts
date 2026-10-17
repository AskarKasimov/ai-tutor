export {
  useSubjectMaterialsQuery,
  useCourseMaterialKindsQuery,
  useUploadCourseMaterialsMutation,
  useReplaceCourseMaterialMutation,
  useUpdateCourseMaterialMetadataMutation,
  useDeleteCourseMaterialMutation,
  useCourseMaterialsChanging,
} from './model/use-competency-map'
export {
  CompetencyMapDependenciesProvider,
  useCompetencyMapDependencies,
} from './model/dependencies-context'
export type { CompetencyMapDependencies } from './model/dependencies-context'

export { MaterialUploadQueue } from './ui/material-upload-queue'
