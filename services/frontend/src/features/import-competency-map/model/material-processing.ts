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

export function materialPreparationStatus(material: CourseMaterial) {
  if (
    material.status === 'failed' ||
    material.processing?.status === 'failed' ||
    material.cleaning?.status === 'failed'
  )
    return 'failed'
  if (material.cleaning?.status === 'pending') return 'cleaningQueued'
  if (material.cleaning?.status === 'processing') return 'cleaning'
  if (materialInProgress(material)) {
    const stage = material.processing?.stage
    if (stage && stage !== 'ready' && stage !== 'failed') return stage
    return material.status === 'pending' ? 'queued' : 'processing'
  }
  return 'ready'
}
