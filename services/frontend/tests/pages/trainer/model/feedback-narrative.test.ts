import { expect, it } from 'vitest'
import { feedbackNarrative } from '@/pages/trainer/model/feedback-narrative'

it('removes follow-up questions and requests while retaining the explanation', () => {
  for (const tail of [
    'Повторите и объясните различие.',
    'Почему это верно?',
    '**Объясните** свой ответ.',
    'Для закрепления сравните методы и приведите пример.',
    'Вам нужно объяснить выбор.',
    'Продолжите со следующим вопросом.',
    'Стоит добавить причину, что целевая переменная двоичная.',
    'Пересмотрите определение классификации и регрессии, уделяя внимание типу целевой переменной.',
  ]) {
    expect(feedbackNarrative(`Тип задачи назван верно. ${tail}`)).toBe(
      'Тип задачи назван верно.',
    )
  }
})

it('keeps descriptive recommendations, decimals, emphasis, and code', () => {
  for (const text of [
    'Вы не объяснили причину. Внимания требует различие типов задач.',
    'Для закрепления полезно повторение **регрессии**. Значение равно 0.5.',
    'В коде используется `x ? a : b`.',
    '    print("Объясните?")',
    '```python\nprint("Объясните?")\n```',
    'Пояснение.\n\n**Различие** связано с типом целевой переменной.',
  ]) {
    expect(feedbackNarrative(text)).toBe(text)
  }
})

it('does not invent a replacement when the old feedback contains only requests', () => {
  expect(feedbackNarrative('Выберите вариант. Объясните выбор.')).toBe('')
})
