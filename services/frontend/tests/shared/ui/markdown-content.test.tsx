import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import { MarkdownContent } from '@/shared/ui'

it('renders inline LaTeX with accessible MathML', () => {
  const { container } = render(
    <MarkdownContent text={'Среднее: $\\frac{a+b}{2}$.'} />,
  )
  expect(container.querySelector('p .katex')).not.toBeNull()
  expect(container.querySelector('math mfrac')).not.toBeNull()
  expect(container.querySelector('annotation')?.textContent).toBe(
    '\\frac{a+b}{2}',
  )
  expect(container.querySelector('.katex-display')).toBeNull()
})

it('renders block LaTeX separately from surrounding prose', () => {
  const { container } = render(
    <MarkdownContent
      text={
        'Ошибка:\n\n$$\n\\sum_{i=1}^{n} (y_i - \\hat{y}_i)^2\n$$\n\nПояснение.'
      }
    />,
  )
  expect(container.querySelector('.katex-display math')).not.toBeNull()
  expect(container.querySelector('pre')).toBeNull()
  expect(screen.getByText('Пояснение.')).toBeVisible()
})

it('preserves math notation inside inline and highlighted code', () => {
  const { container } = render(
    <MarkdownContent
      text={'Пример: `$x^2$`\n\n```python\nformula = "$x^2$"\n```'}
    />,
  )
  expect(container.querySelector('p code')?.textContent).toBe('$x^2$')
  expect(container.querySelector('pre')?.textContent).toBe(
    'formula = "$x^2$"\n',
  )
  expect(container.querySelector('.katex')).toBeNull()
  expect(container.querySelector('pre .hljs-string')).not.toBeNull()
})

it('repairs doubly escaped LaTeX commands but keeps real line breaks', () => {
  const { container } = render(
    <MarkdownContent
      text={
        'Модель: $\\\\hat y = 2.5\\\\,x_{\\\\text{м²}}$.\n\n$$\na \\\\ b\n$$'
      }
    />,
  )
  const [inline, block] = Array.from(
    container.querySelectorAll('annotation'),
  ).map((node) => node.textContent)
  expect(inline).toBe('\\hat y = 2.5\\,x_{\\text{м²}}')
  expect(block).toBe('a \\\\ b')
  expect(container.querySelector('.katex-error')).toBeNull()
  // Code keeps its backslashes untouched.
  const code = render(<MarkdownContent text={'`\\\\beta`'} />).container
  expect(code.querySelector('code')?.textContent).toBe('\\\\beta')
})

it('keeps invalid LaTeX readable without losing the surrounding text', () => {
  const { container } = render(
    <MarkdownContent text={'Пример: $\\frac{$\n\nСледующий абзац.'} />,
  )
  expect(container.querySelector('.katex-error')?.textContent).toBe('\\frac{')
  expect(screen.getByText('Следующий абзац.')).toBeVisible()
})

it('keeps escaped currency as plain text', () => {
  const { container } = render(
    <MarkdownContent text={'Стоимость: \\$5 и \\$10.'} />,
  )
  expect(screen.getByText('Стоимость: $5 и $10.')).toBeVisible()
  expect(container.querySelector('.katex')).toBeNull()
})

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

it('normalizes code blocks glued directly to preceding text on the same line', () => {
  const { container } = render(
    <MarkdownContent
      text={'Пример в Python: ```python\nimport pandas as pd\nx = 1\n```'}
    />,
  )
  const pre = container.querySelector('pre')
  expect(pre).not.toBeNull()
  expect(pre?.textContent).toContain('import pandas as pd')
  const p = container.querySelector('p')
  expect(p).not.toBeNull()
  expect(p?.textContent).toBe('Пример в Python:')
})

it.each([
  ['Python string', '```python', 'marker = "```"\nprint(marker)\n', '```'],
  ['four-backtick fence', '````text', '```python\nx = 1\n```\n', '````'],
  ['tilde fence', '~~~python', 'marker = "```"\n', '~~~'],
  ['unclosed fence', '```python', 'marker = "```"\n', ''],
  ['longer closing fence', '```python', 'x = 1\n', '````'],
])('preserves code inside a %s', (_name, opening, code, closing) => {
  const { container } = render(
    <MarkdownContent text={`${opening}\n${code}${closing}`} />,
  )
  expect(container.querySelector('pre')?.textContent).toBe(code)
})

it('preserves inline code delimited with triple backticks', () => {
  const { container } = render(
    <MarkdownContent text={'Используйте ```x ` y``` в примере.'} />,
  )
  expect(container.querySelector('pre')).toBeNull()
  expect(container.querySelector('code')?.textContent).toBe('x ` y')
})

it('normalizes a glued fence after a closed code block', () => {
  const { container } = render(
    <MarkdownContent
      text={'```python\nx = 1\n````\n\nСледующий пример: ```python\ny = 2\n```'}
    />,
  )
  expect(
    Array.from(container.querySelectorAll('pre'), (pre) => pre.textContent),
  ).toEqual(['x = 1\n', 'y = 2\n'])
})

it.each([
  ['python', 'def greet():\n    return "hello"', 'def'],
  ['py', 'def greet():\n    return "hello"', 'def'],
  ['javascript', 'const answer = 42;', 'const'],
  ['js', 'const answer = 42;', 'const'],
  ['typescript', 'interface User { name: string }', 'interface'],
  ['sql', 'SELECT name FROM users;', 'SELECT'],
  ['go', 'func main() {}', 'func'],
])(
  'highlights %s code without changing its text',
  (language, code, keyword) => {
    const { container } = render(
      <MarkdownContent text={`\`\`\`${language}\n${code}\n\`\`\``} />,
    )
    const pre = container.querySelector('pre')
    expect(pre?.textContent).toBe(`${code}\n`)
    expect(
      Array.from(
        pre?.querySelectorAll('.hljs-keyword') ?? [],
        (node) => node.textContent,
      ),
    ).toContain(keyword)
    expect(container.querySelector('pre pre')).toBeNull()
    expect(pre?.querySelector('[style]')).toBeNull()
    expect(pre).not.toHaveAttribute('style')
  },
)

it.each(['', 'unknown-language', 'text'])(
  'keeps %s blocks plain',
  (language) => {
    const { container } = render(
      <MarkdownContent
        text={`\`\`\`${language}\nconst answer = "a\\nb";\n\`\`\``}
      />,
    )
    const pre = container.querySelector('pre')
    expect(pre?.textContent).toBe('const answer = "a\\nb";\n')
    expect(pre?.querySelector('span')).toBeNull()
  },
)

it('keeps inline code plain next to a highlighted block', () => {
  const { container } = render(
    <MarkdownContent
      text={'Используйте `const x = 1`:\n\n```js\nconst x = 1\n```'}
    />,
  )
  const inline = container.querySelector('p code')
  expect(inline?.textContent).toBe('const x = 1')
  expect(inline?.querySelector('span')).toBeNull()
  expect(container.querySelector('pre .hljs-keyword')).toHaveTextContent(
    'const',
  )
})

it('highlights HTML as code without creating executable elements', () => {
  const { container } = render(
    <MarkdownContent text={'```html\n<script>alert(1)</script>\n```'} />,
  )
  expect(container.querySelector('pre')?.textContent).toBe(
    '<script>alert(1)</script>\n',
  )
  expect(container.querySelector('pre .hljs-name')).toHaveTextContent('script')
  expect(container.querySelector('script')).toBeNull()
})
