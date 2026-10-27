import { expect, it } from 'vitest'
import { buildDiagnosticGraph } from '@/pages/learning-trainer/model/diagnostic-graph'

it('groups selected constituents under competencies without inventing prerequisite edges', () => {
  expect(
    buildDiagnosticGraph({
      competencies: [
        {
          id: 'c1',
          name: 'Компетенция',
          constituents: [
            {
              id: 't1',
              name: 'Выбранная составляющая',
              outcomes: [
                { id: 'main', name: 'Основной ОР' },
                { id: 'basic', name: 'Базовый ОР' },
              ],
            },
          ],
        },
      ],
    }),
  ).toEqual({
    topics: [
      { id: 'competency:c1', title: 'Компетенция' },
      {
        id: 'constituent:t1',
        title: 'Выбранная составляющая',
        provides: ['Основной ОР', 'Базовый ОР'],
      },
    ],
    hierarchy: [{ source: 'competency:c1', target: 'constituent:t1' }],
    prerequisites: [],
  })
})
