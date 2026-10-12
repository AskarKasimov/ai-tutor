import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import { FormattedQuestionContent } from '@/pages/trainer/ui/trainer-shared'

it('shows a plain task as the heading', () => {
  render(<FormattedQuestionContent question="Что такое регрессия?" />)
  expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
    'Что такое регрессия?',
  )
})

it('renders a Markdown table under the lead text', () => {
  render(
    <FormattedQuestionContent
      headingLevel="h3"
      question={[
        'Лог обучения с max_depth = 8:',
        '| Итерация | Train Loss | Val Loss |',
        '|---|:---:|---:|',
        '| 50 | 0.32 | 0.35 |',
        '| 150 | 0.14 | 0.21 |',
      ].join('\n')}
    />,
  )
  expect(screen.getByRole('heading', { level: 3 })).toHaveTextContent(
    'Лог обучения с max_depth = 8:',
  )
  const table = screen.getByRole('table')
  expect(within(table).getAllByRole('columnheader')).toHaveLength(3)
  expect(within(table).getAllByRole('row')).toHaveLength(3)
  expect(within(table).getByText('0.21')).toHaveStyle({ textAlign: 'right' })
})

it('keeps lists, line breaks and formula symbols of the bank tasks', () => {
  const { container } = render(
    <FormattedQuestionContent
      question={[
        'Все признаки стандартизированы.',
        'ŷ = 0.8 · Площадь - 0.5 · Расстояние_до_метро + b',
        'grad_w = (2 / len(X)) * X.T @ error, grad_b = 2 * error.mean()',
        '',
        'Требования:',
        '1. Гладкость около нуля.',
        '2. Линейный рост штрафа.',
        '- Итерация 1: MSE = 24.5',
      ].join('\n')}
    />,
  )
  const heading = screen.getByRole('heading', { level: 1 })
  // Single line breaks stay; underscores and asterisks are not emphasis.
  expect(heading.querySelectorAll('br')).toHaveLength(2)
  expect(heading).toHaveTextContent('Расстояние_до_метро')
  expect(heading).toHaveTextContent('(2 / len(X)) * X.T @ error')
  expect(container.querySelector('em, strong')).toBeNull()
  expect(screen.getAllByRole('list')).toHaveLength(2)
  expect(screen.getByText('Линейный рост штрафа.')).toBeVisible()
})

it('does not render raw HTML from the task text', () => {
  const { container } = render(
    <FormattedQuestionContent
      question={'Вопрос <img src=x onerror="alert(1)">'}
    />,
  )
  expect(container.querySelector('img')).toBeNull()
})

it('renders code blocks in pre elements with monospaced code content', () => {
  const { container } = render(
    <FormattedQuestionContent
      question={[
        'Изучите код:',
        '```python',
        'import numpy as np',
        'X = np.array([1, 2])',
        '```',
      ].join('\n')}
    />,
  )
  const pre = container.querySelector('pre')
  expect(pre).not.toBeNull()
  expect(pre).toHaveTextContent('import numpy as np')
  expect(pre).toHaveTextContent('X = np.array([1, 2])')
})
