import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import { MarkdownContent } from '@/shared/ui'

it.each(['python', ''])(
  'keeps %s code separate from the following table',
  (language) => {
    const { container } = render(
      <MarkdownContent
        text={[
          '**Пример** с `x`:',
          '',
          `\`\`\`${language}`,
          'for step in range(2):',
          '    print("a\\nb")',
          '```',
          '',
          '| Шаг | Loss |',
          '|---|---:|',
          '| 1 | 0.5 |',
        ].join('\n')}
      />,
    )
    expect(container.querySelector('pre')?.textContent).toBe(
      'for step in range(2):\n    print("a\\nb")\n',
    )
    const table = screen.getByRole('table')
    expect(within(table).getByText('0.5')).toHaveStyle({ textAlign: 'right' })
    expect(container.querySelector('pre table')).toBeNull()
    expect(screen.getByText('x').tagName).toBe('CODE')
    expect(screen.queryByRole('heading')).toBeNull()
  },
)

it('keeps raw HTML and unsafe links inactive', () => {
  const { container } = render(
    <MarkdownContent
      text={
        '<img src=x onerror="alert(1)">\n\n[ссылка](javascript:alert%281%29)'
      }
    />,
  )
  expect(container.querySelector('img')).toBeNull()
  expect(screen.getByText('ссылка')).not.toHaveAttribute(
    'href',
    expect.stringContaining('javascript:'),
  )
})
