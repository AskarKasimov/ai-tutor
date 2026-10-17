import type { CourseMaterial } from '@/entities/competency-map'
export function materialInProgress(material: CourseMaterial): boolean {
  const active = (status?: string) =>
    status === 'pending' || status === 'processing'
  if (material.status === 'failed') return false
  return (
    material.status === 'pending' ||
    active(material.processing?.status) ||
    active(material.cleaning?.status)
  )
}
export function materialsRefetchInterval(
  materials: CourseMaterial[] | undefined,
): number | false {
  return materials?.some(materialInProgress) ? 4000 : false
}
