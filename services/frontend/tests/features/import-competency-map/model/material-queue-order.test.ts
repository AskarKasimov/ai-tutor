import { expect, it } from 'vitest'
import {
  moveMaterialQueueItem,
  moveMaterialQueueItemToIndex,
  sortMaterialsByWeek,
} from '@/features/import-competency-map/model/material-queue-order'

const items = [
  { id: 'week-10', week: '10' },
  { id: 'week-2a', week: '2' },
  { id: 'week-2b', week: '2' },
  { id: 'unset', week: '' },
]

it('sorts weeks numerically and preserves order for equal or unset weeks', () => {
  expect(sortMaterialsByWeek(items).map((item) => item.id)).toEqual([
    'week-2a',
    'week-2b',
    'week-10',
    'unset',
  ])
})

it('moves an item to the dropped position', () => {
  const moved = moveMaterialQueueItem(items, 'week-10', 'unset')

  expect(moved.map((item) => item.id)).toEqual([
    'week-2a',
    'week-2b',
    'week-10',
    'unset',
  ])
  expect(moved.find((item) => item.id === 'week-10')?.week).toBe('2')
})

it('uses the next row week when a moved item becomes first', () => {
  const moved = moveMaterialQueueItem(
    [
      { id: 'week-1', week: '1' },
      { id: 'week-2', week: '2' },
      { id: 'week-3', week: '3' },
    ],
    'week-3',
    'week-1',
  )

  expect(moved.map(({ id, week }) => [id, week])).toEqual([
    ['week-3', '1'],
    ['week-1', '1'],
    ['week-2', '2'],
  ])
})

it('keeps the order when either item is missing', () => {
  expect(moveMaterialQueueItem(items, 'missing', 'week-2a')).toEqual(items)
})

it('moves an item to an exact index for keyboard reordering', () => {
  expect(
    moveMaterialQueueItemToIndex(items, 'week-10', 1).map((item) => item.id),
  ).toEqual(['week-2a', 'week-10', 'week-2b', 'unset'])
})
