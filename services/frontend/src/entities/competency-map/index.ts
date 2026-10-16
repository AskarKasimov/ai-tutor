export type { CourseMaterial, CourseMaterialKind } from './model/competency-map'
export { CompetencyMapApiError } from './model/competency-map-error'
export {
  readSubjectMaterials,
  uploadCourseMaterials,
  updateCourseMaterialMetadata,
  deleteCourseMaterial,
  downloadCourseMaterialRaw,
} from './api/competency-map-api'
export type {
  CourseMaterialUploadFailedItem,
  CourseMaterialUploadResult,
  CourseMaterialMetadataPatch,
} from './api/competency-map-api'
