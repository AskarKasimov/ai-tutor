import { expect, it } from 'vitest'
import { groupWarnings } from '@/pages/competency-map/model/import-warnings'

it('groups warnings by row and merges columns with the same reason', () => {
  expect(
    groupWarnings([
      { row: 9, columnIndex: 11, column: 'Задание 1', code: 'TASK_EMPTY_PART' },
      {
        row: 6,
        columnIndex: 13,
        column: 'Задание 3',
        code: 'TASK_MISSING_SCREEN',
      },
      {
        row: 6,
        columnIndex: 11,
        column: 'Задание 1',
        code: 'TASK_MISSING_VOICE',
      },
      {
        row: 6,
        columnIndex: 12,
        column: 'Задание 2',
        code: 'TASK_MISSING_SCREEN',
      },
    ]),
  ).toEqual([
    {
      row: 6,
      reasons: [
        { code: 'TASK_MISSING_VOICE', columns: ['Задание 1'] },
        { code: 'TASK_MISSING_SCREEN', columns: ['Задание 2', 'Задание 3'] },
      ],
    },
    {
      row: 9,
      reasons: [{ code: 'TASK_EMPTY_PART', columns: ['Задание 1'] }],
    },
  ])
})
