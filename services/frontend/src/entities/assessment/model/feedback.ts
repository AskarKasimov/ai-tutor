export function normalizeText(text: string): string {
  if (!text) return ''
  return text
    .replace(/\\r\\n/g, '\n')
    .replace(/\\n/g, '\n')
    .replace(/\r\n/g, '\n')
    .trim()
}

interface ParsedGap {
  title: string
  explanation: string
  subtopics: string[]
}

export function parseGapItem(raw: string): ParsedGap {
  const normalized = normalizeText(raw)
  const lines = normalized
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  if (lines.length === 0) {
    return { title: raw, explanation: '', subtopics: [] }
  }

  let title = lines[0]
  let explanation = ''
  const subtopics: string[] = []
  let inRepeatSection = false

  if (lines.length === 1) {
    const single = lines[0]
    const repeatMatch = single.match(
      /(?:(?:•|\-|\—|\*|\s)*(?:Что повторить(?:\s*по теме)?:\s*))(.*)$/i,
    )
    const mistakeMatch = single.match(
      /(?:(?:•|\-|\—|\*|\s)*(?:Пояснение|В чём ошибка):\s*)(.*?)(?=(?:(?:•|\-|\—|\*|\s)*Что повторить|$))/i,
    )

    if (repeatMatch || mistakeMatch) {
      if (mistakeMatch) {
        explanation = mistakeMatch[1].trim()
      }
      if (repeatMatch) {
        const repeatContent = repeatMatch[1].trim()
        const bullets = repeatContent
          .split(/[;•·]\s*/)
          .map((b) => b.trim())
          .filter(Boolean)
        for (const b of bullets) {
          const clean = b
            .replace(/^[•\-\—\*\s]+/, '')
            .trim()
            .replace(/[.;]+$/, '')
          if (clean) subtopics.push(clean)
        }
      }
      const titleEndIndex = single.search(
        /(?:•|\-|\—|\*|\s)*(?:Пояснение|В чём ошибка|Что повторить)/i,
      )
      if (titleEndIndex > 0) {
        title = single
          .slice(0, titleEndIndex)
          .trim()
          .replace(/[.;\s]+$/, '')
      }
      return { title, explanation, subtopics }
    }

    return { title: single, explanation: '', subtopics: [] }
  }

  for (let i = 1; i < lines.length; i++) {
    const rawLine = lines[i]
    const stripped = rawLine.replace(/^[•\-\—\*\s]+/, '').trim()
    if (!stripped) continue

    const lower = stripped.toLowerCase()

    if (lower.startsWith('что повторить')) {
      inRepeatSection = true
      const inlineAfter = stripped
        .replace(/^что повторить(?:\s*по теме)?:\s*/i, '')
        .trim()
      if (inlineAfter) {
        const clean = inlineAfter
          .replace(/^[•\-\—\*\s]+/, '')
          .trim()
          .replace(/[.;]+$/, '')
        if (clean) subtopics.push(clean)
      }
      continue
    }

    if (lower.startsWith('пояснение:') || lower.startsWith('в чём ошибка:')) {
      inRepeatSection = false
      explanation = stripped
        .replace(/^(?:пояснение|в чём ошибка):\s*/i, '')
        .trim()
        .replace(/[.;]+$/, '')
      continue
    }

    if (inRepeatSection) {
      const clean = stripped
        .replace(/^[•\-\—\*\s]+/, '')
        .trim()
        .replace(/[.;]+$/, '')
      if (clean) subtopics.push(clean)
    } else if (!explanation) {
      explanation = stripped.replace(/[.;]+$/, '')
    } else {
      const clean = stripped
        .replace(/^[•\-\—\*\s]+/, '')
        .trim()
        .replace(/[.;]+$/, '')
      if (clean) subtopics.push(clean)
    }
  }

  return { title, explanation, subtopics }
}

export function extractCleanTopicTitle(title: string): string {
  const match = title.match(/«([^»]+)»/)
  if (match) return match[1]
  return title
    .replace(/^(Тема|тема)\s*/i, '')
    .replace(/—\s*(выявлен пробел|частично освоена|частичный ответ).*$/i, '')
    .trim()
}

interface ParsedPartial {
  title: string
  note: string
}

export function parsePartialItem(raw: string): ParsedPartial {
  const normalized = normalizeText(raw)
  const lines = normalized
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  if (lines.length === 0) return { title: raw, note: '' }

  let title = lines[0]
  let note = ''

  for (let i = 1; i < lines.length; i++) {
    const line = lines[i]
    if (
      line.toLowerCase().startsWith('• замечание:') ||
      line.toLowerCase().startsWith('замечание:')
    ) {
      note = line.replace(/^[•\-\—\s]*замечание:\s*/i, '').trim()
      break
    }
    if (
      line.toLowerCase().startsWith('пояснение:') ||
      line.toLowerCase().startsWith('комментарий:')
    ) {
      note = line.replace(/^[•\-\—\s]*(пояснение|комментарий):\s*/i, '').trim()
      break
    }
    if (!note) {
      note = line.replace(/^[•\-\—\s]+/, '').trim()
    }
  }

  if (!note && title.includes('Причина:')) {
    const parts = title.split('Причина:')
    title = parts[0].trim()
    note = parts[1].trim()
  }

  return { title, note }
}

interface ExpressStep {
  title: string
  note: string
}

export function getExpressSteps(
  gaps: string[],
  partials: string[],
  recommendations: string[],
): ExpressStep[] {
  const steps: ExpressStep[] = []

  if (gaps.length > 0) {
    const gapTopics = gaps.map((g) =>
      extractCleanTopicTitle(parseGapItem(g).title),
    )
    const firstGap = parseGapItem(gaps[0])
    const focus =
      firstGap.subtopics.slice(0, 2).join(', ') ||
      'базовые определения и различия'
    steps.push({
      title: `Ликвидировать ${gaps.length} критических пробела(ов) (0/2)`,
      note: `Темы: ${gapTopics.map((t) => `«${t}»`).join(', ')}. Фокус: ${focus}.`,
    })
  }

  if (partials.length > 0) {
    const partialTopics = partials.map((p) =>
      extractCleanTopicTitle(parsePartialItem(p).title),
    )
    steps.push({
      title: `Доработать ${partials.length} частично освоенных тем(ы) (1/2)`,
      note: `Темы: ${partialTopics.map((t) => `«${t}»`).join(', ')}. Добавьте развёрнутое обоснование и критерии выбора.`,
    })
  } else if (gaps.length === 0 && recommendations.length > 0) {
    steps.push({
      title: 'Углубить знания по продвинутым темам',
      note: normalizeText(recommendations[0]),
    })
  }

  steps.push({
    title: 'Пройти тренировочную сессию',
    note: 'Закрепить результат на 2–3 практических заданиях и выйти на 14/14.',
  })

  return steps
}
