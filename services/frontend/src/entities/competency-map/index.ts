export type {
  CourseMaterial,
  CourseMaterialKind,
  CourseMaterialKindOption,
} from './model/competency-map'
export { CompetencyMapApiError } from './model/competency-map-error'
export {
  readSubjectMaterials,
  readCourseMaterialKinds,
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
