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

export {
  MaterialReorderControls,
  MaterialUploadQueue,
  MaterialUploadQueueToolbar,
  MaterialUploadSubmit,
  MaterialQueueCard,
} from './ui/material-upload-queue'
export { CourseMaterialCard } from './ui/course-material-card'
export { MaterialMetadataFields } from './ui/material-metadata-fields'
export { useMaterialUploadQueue } from './model/use-material-upload-queue'
export type { MaterialQueueItem } from './model/use-material-upload-queue'
export {
  moveMaterialQueueItem,
  moveMaterialQueueItemToIndex,
  sortMaterialsByWeek,
} from './model/material-queue-order'

export {
  materialInProgress,
  materialsRefetchInterval,
} from './model/material-processing'
