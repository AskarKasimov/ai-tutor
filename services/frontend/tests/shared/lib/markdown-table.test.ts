import { describe, expect, it } from 'vitest'
import { parseQuestionContent } from '@/shared/lib/markdown-table'

describe('parseQuestionContent', () => {
  it('returns empty array on empty input', () => {
    expect(parseQuestionContent('')).toEqual([])
  })

  it('parses plain text without tables as a single text block', () => {
    const text = 'Какая из функций потерь более устойчива к выбросам?'
    const blocks = parseQuestionContent(text)
    expect(blocks).toEqual([{ type: 'text', text }])
  })

  it('parses text with a markdown table', () => {
    const raw = `Таблица данных автосалона:
| ID | Марка | Пробег, тыс. км | Год выпуска |
|---|---|---|---|
| 101 | Toyota | 85 | 2018 |
| 102 | BMW | 120 | 2016 |`

    const blocks = parseQuestionContent(raw)
    expect(blocks).toHaveLength(2)
    expect(blocks[0]).toEqual({
      type: 'text',
      text: 'Таблица данных автосалона:',
    })
    expect(blocks[1]).toEqual({
      type: 'table',
      data: {
        headers: ['ID', 'Марка', 'Пробег, тыс. км', 'Год выпуска'],
        alignments: ['left', 'left', 'left', 'left'],
        rows: [
          ['101', 'Toyota', '85', '2018'],
          ['102', 'BMW', '120', '2016'],
        ],
      },
    })
  })

  it('parses table alignments correctly', () => {
    const raw = `| Название | Значение | Доля |
|:---|:---:|---:|
| Модель А | 42 | 15% |`

    const blocks = parseQuestionContent(raw)
    expect(blocks).toHaveLength(1)
    if (blocks[0].type === 'table') {
      expect(blocks[0].data.alignments).toEqual(['left', 'center', 'right'])
      expect(blocks[0].data.headers).toEqual(['Название', 'Значение', 'Доля'])
      expect(blocks[0].data.rows).toEqual([['Модель А', '42', '15%']])
    }
  })

  it('parses text before table, table, and outro text after table', () => {
    const raw = `Линейная регрессия обучается:
| Итерация | Train MSE | Val MSE |
|---|---|---|
| 50 | 14.2 | 14.5 |
| 200 | 12.8 | 13.0 |
Ошибки перестали снижаться и зафиксировались на высоком уровне.`

    const blocks = parseQuestionContent(raw)
    expect(blocks).toHaveLength(3)
    expect(blocks[0]).toEqual({
      type: 'text',
      text: 'Линейная регрессия обучается:',
    })
    expect(blocks[1].type).toBe('table')
    expect(blocks[2]).toEqual({
      type: 'text',
      text: 'Ошибки перестали снижаться и зафиксировались на высоком уровне.',
    })
  })
})
