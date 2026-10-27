import { describe, expect, it } from 'vitest'
import {
  cleanCompetencyTitle,
  cleanTopicTitle,
  layoutDiagnosticMindmap,
  SIRIUS_BRANCH_COLORS,
} from '@/pages/learning-trainer/model/diagnostic-mindmap'

describe('cleanCompetencyTitle', () => {
  it('strips "К1:", "К-1", "К1.", "К1 " and "Компетенция X:" prefixes', () => {
    expect(cleanCompetencyTitle('К1: Линейная алгебра')).toBe(
      'Линейная алгебра',
    )
    expect(cleanCompetencyTitle('К1. Математический анализ')).toBe(
      'Математический анализ',
    )
    expect(cleanCompetencyTitle('К-2 Дискретные структуры')).toBe(
      'Дискретные структуры',
    )
    expect(cleanCompetencyTitle('К 3: Теория вероятностей')).toBe(
      'Теория вероятностей',
    )
    expect(cleanCompetencyTitle('Компетенция 1: Алгоритмы')).toBe('Алгоритмы')
    expect(cleanCompetencyTitle('1. Основы ML')).toBe('Основы ML')
  })

  it('preserves clean title without prefixes or when only code is given', () => {
    expect(cleanCompetencyTitle('Машинное обучение')).toBe('Машинное обучение')
    expect(cleanCompetencyTitle('К1')).toBe('К1')
  })
})

describe('cleanTopicTitle', () => {
  it('strips numeric prefixes like "1.1.", "1.1", "1.", "1)", and "Тема X" prefixes', () => {
    expect(cleanTopicTitle('1.1. Линейная регрессия')).toBe(
      'Линейная регрессия',
    )
    expect(cleanTopicTitle('1.1 Градиентный спуск')).toBe('Градиентный спуск')
    expect(cleanTopicTitle('1. Решающие деревья')).toBe('Решающие деревья')
    expect(cleanTopicTitle('2) Случайный лес')).toBe('Случайный лес')
    expect(cleanTopicTitle('Тема 3. Бустинг над деревьями')).toBe(
      'Бустинг над деревьями',
    )
    expect(cleanTopicTitle('Тема 4: Нейронные сети')).toBe('Нейронные сети')
  })

  it('preserves titles without numeric prefixes', () => {
    expect(cleanTopicTitle('Линейная регрессия')).toBe('Линейная регрессия')
    expect(cleanTopicTitle('1.1')).toBe('1.1')
  })
})

describe('layoutDiagnosticMindmap', () => {
  it('places the subject root node in the center when outline has no competencies', () => {
    const map = layoutDiagnosticMindmap(
      { competencies: [] },
      'Машинное обучение',
    )
    expect(map.nodes).toHaveLength(1)
    expect(map.nodes[0]).toMatchObject({
      id: 'subject-root',
      type: 'subject',
      data: { kind: 'subject', title: 'Машинное обучение' },
    })
    expect(map.edges).toHaveLength(0)
  })

  it('branches out competencies and constituents in Sirius radial tree style', () => {
    const map = layoutDiagnosticMindmap(
      {
        competencies: [
          {
            id: 'c1',
            name: 'Компетенция 1',
            constituents: [
              {
                id: 't1',
                name: 'Составляющая 1.1',
                outcomes: [
                  { id: 'o1', name: 'ОР 1', role: 'main' },
                  { id: 'o2', name: 'ОР 2', role: 'basic' },
                ],
              },
            ],
          },
          {
            id: 'c2',
            name: 'Компетенция 2',
            constituents: [
              {
                id: 't2',
                name: 'Составляющая 2.1',
                outcomes: [{ id: 'o3', name: 'ОР 3', role: 'main' }],
              },
            ],
          },
        ],
      },
      'Физика',
      't1',
    )

    // Center Subject + 2 Competencies + 2 Constituents = 5 nodes
    expect(map.nodes).toHaveLength(5)

    const subject = map.nodes.find((n) => n.id === 'subject-root')
    const comp1 = map.nodes.find((n) => n.id === 'competency:c1')
    const comp2 = map.nodes.find((n) => n.id === 'competency:c2')
    const const1 = map.nodes.find((n) => n.id === 'constituent:t1')
    const const2 = map.nodes.find((n) => n.id === 'constituent:t2')

    expect(subject?.data.title).toBe('Физика')

    // Competency 1 is on the right side
    expect(comp1?.position.x).toBeGreaterThan(0)
    expect(const1?.position.x).toBeGreaterThan(comp1?.position.x ?? 0)

    // Competency 2 is on the left side
    expect(comp2?.position.x).toBeLessThan(0)
    expect(const2?.position.x).toBeLessThan(comp2?.position.x ?? 0)

    // Constituent 1 is selected
    expect(const1?.data).toMatchObject({
      kind: 'constituent',
      id: 't1',
      isSelected: true,
      outcomes: [
        { id: 'o1', name: 'ОР 1', role: 'main' },
        { id: 'o2', name: 'ОР 2', role: 'basic' },
      ],
    })

    // Constituent 2 is not selected
    expect(const2?.data).toMatchObject({
      kind: 'constituent',
      id: 't2',
      isSelected: false,
    })

    // Edges connect Subject -> Competency -> Constituent
    expect(map.edges).toHaveLength(4)
    expect(map.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: 'subject-root',
          target: 'competency:c1',
          style: expect.objectContaining({
            stroke: SIRIUS_BRANCH_COLORS[0].color,
          }),
        }),
        expect.objectContaining({
          source: 'competency:c1',
          target: 'constituent:t1',
          style: expect.objectContaining({
            stroke: SIRIUS_BRANCH_COLORS[0].color,
          }),
        }),
        expect.objectContaining({
          source: 'subject-root',
          target: 'competency:c2',
          style: expect.objectContaining({
            stroke: SIRIUS_BRANCH_COLORS[1].color,
          }),
        }),
        expect.objectContaining({
          source: 'competency:c2',
          target: 'constituent:t2',
          style: expect.objectContaining({
            stroke: SIRIUS_BRANCH_COLORS[1].color,
          }),
        }),
      ]),
    )
  })
})
