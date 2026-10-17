type QueueItem = { id: string }
export function moveMaterialQueueItem<T extends QueueItem>(
  items: readonly T[],
  draggedId: string,
  targetId: string,
): T[] {
  if (draggedId === targetId) return [...items]
  const sourceIndex = items.findIndex((item) => item.id === draggedId)
  if (sourceIndex < 0 || !items.some((item) => item.id === targetId))
    return [...items]
  const reordered = [...items]
  const [dragged] = reordered.splice(sourceIndex, 1)
  reordered.splice(
    reordered.findIndex((item) => item.id === targetId),
    0,
    dragged,
  )
  return reordered
}
export function moveMaterialQueueItemToIndex<T extends QueueItem>(
  items: readonly T[],
  movedId: string,
  targetIndex: number,
): T[] {
  const sourceIndex = items.findIndex((item) => item.id === movedId)
  if (sourceIndex < 0 || targetIndex < 0 || targetIndex >= items.length)
    return [...items]
  const reordered = [...items]
  const [moved] = reordered.splice(sourceIndex, 1)
  reordered.splice(targetIndex, 0, moved)
  return reordered
}
