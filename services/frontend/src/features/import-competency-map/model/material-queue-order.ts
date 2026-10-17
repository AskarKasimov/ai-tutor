type WeekItem = { id: string; week: string }

function weekNumber(week: string) {
  const value = Number(week)
  return week.trim() && Number.isFinite(value) && value > 0
    ? value
    : Number.POSITIVE_INFINITY
}

export function sortMaterialsByWeek<T extends WeekItem>(
  items: readonly T[],
): T[] {
  return items
    .map((item, index) => ({ item, index }))
    .sort(
      (a, b) =>
        weekNumber(a.item.week) - weekNumber(b.item.week) || a.index - b.index,
    )
    .map(({ item }) => item)
}

export function moveMaterialQueueItem<T extends { id: string }>(
  items: readonly T[],
  draggedId: string,
  targetId: string,
): T[] {
  const sourceIndex = items.findIndex((item) => item.id === draggedId)
  if (sourceIndex < 0 || draggedId === targetId) return [...items]

  const reordered = [...items]
  const [dragged] = reordered.splice(sourceIndex, 1)
  const targetIndex = reordered.findIndex((item) => item.id === targetId)
  if (targetIndex < 0) return [...items]
  reordered.splice(targetIndex, 0, dragged)
  return reordered
}

export function moveMaterialQueueItemToIndex<T extends { id: string }>(
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
