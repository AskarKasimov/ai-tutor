import { expect, it } from 'vitest'
import { splitLessonParagraphs } from '@/pages/trainer/model/lesson-paragraphs'

it('renders literal escaped paragraph separators in saved lessons', () => {
  expect(splitLessonParagraphs('Первый абзац.\\n\\nВторой абзац.')).toEqual([
    'Первый абзац.',
    'Второй абзац.',
  ])
  expect(splitLessonParagraphs('Первый.\r\n\r\nВторой.')).toEqual([
    'Первый.',
    'Второй.',
  ])
})

it('preserves code escapes and LaTeX commands', () => {
  const text = 'Команда `print("\\n\\n")` и формула \\nu + \\nabla f.'
  expect(splitLessonParagraphs(text)).toEqual([text])
  const block = '```python\nprint("\\n\\n")\n```'
  expect(splitLessonParagraphs(block)).toEqual([block])
})
