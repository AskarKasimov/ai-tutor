import { expect, it } from 'vitest'
import {
  moveMaterialQueueItem,
  moveMaterialQueueItemToIndex,
} from '@/features/import-competency-map/model/material-queue-order'
const items = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]
it('moves an item to the dropped position without mutating the source', () => {
  expect(moveMaterialQueueItem(items, 'a', 'c').map(({ id }) => id)).toEqual([
    'b',
    'a',
    'c',
  ])
  expect(items.map(({ id }) => id)).toEqual(['a', 'b', 'c'])
})
it('keeps order if source or target is missing', () => {
  expect(moveMaterialQueueItem(items, 'missing', 'a')).toEqual(items)
  expect(moveMaterialQueueItem(items, 'a', 'missing')).toEqual(items)
})
it('moves an item to an exact keyboard position', () => {
  expect(
    moveMaterialQueueItemToIndex(items, 'a', 2).map(({ id }) => id),
  ).toEqual(['b', 'c', 'a'])
  expect(moveMaterialQueueItemToIndex(items, 'a', -1)).toEqual(items)
})
