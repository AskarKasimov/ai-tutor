export type { CourseMaterial, CourseMaterialKind } from './model/competency-map'
export { CompetencyMapApiError } from './model/competency-map-error'
export {
  readSubjectMaterials,
  uploadCourseMaterials,
  updateCourseMaterialMetadata,
  deleteCourseMaterial,
  replaceCourseMaterialFile,
  downloadCourseMaterialRaw,
} from './api/competency-map-api'
export type {
  CourseMaterialUploadFailedItem,
  CourseMaterialUploadItem,
  CourseMaterialUploadItemResult,
  CourseMaterialUploadResult,
  CourseMaterialMetadataPatch,
} from './api/competency-map-api'
