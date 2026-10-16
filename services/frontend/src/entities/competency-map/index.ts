export type { CourseMaterial } from './model/competency-map'
export { CompetencyMapApiError } from './model/competency-map-error'
export {
  readSubjectMaterials,
  uploadCourseMaterials,
  deleteCourseMaterial,
  downloadCourseMaterialRaw,
} from './api/competency-map-api'
export type {
  CourseMaterialUploadFailedItem,
  CourseMaterialUploadResult,
} from './api/competency-map-api'
