import { i18n } from '@/shared/i18n'

// Stored reports include the constituent in every outcome's advice. Only split
// the known report prefixes; arbitrary model text is preserved unchanged.
export function groupFeedbackDetails(details: string[]) {
  const groups = new Map<string, string[]>()
  for (const text of details) {
    const match = text.match(
      /^(Необходимо изучить полностью составляющую «[^»]+»\.|Дополнительно изучить\. Составляющая «[^»]+»,|Вопрос пропущен: необходимо проверить и отработать результат\. Составляющая «[^»]+»,)\s*/,
    )
    const topic = match?.[1].match(/«([^»]+)»/)?.[1] ?? ''
    const heading = match
      ? i18n.t(
          match[1].startsWith('Необходимо')
            ? 'diagnostic.feedbackStudyTopic'
            : match[1].startsWith('Вопрос пропущен')
              ? 'diagnostic.feedbackSkippedTopic'
              : 'diagnostic.feedbackReviewTopic',
          {
            topic: topic
              .replace(/^\d+(?:\.\d+)+[.)]?\s+/, '')
              .replace(/^Умеет\s+/i, ''),
          },
        )
      : ''
    const body = match
      ? text
          .slice(match[0].length)
          .replace(/^(?:Проверенный результат:\s*|результат\s*)«([^»]+)»/, '$1')
      : text
    const items = groups.get(heading)
    if (items) items.push(body)
    else groups.set(heading, [body])
  }
  return [...groups].map(([heading, items]) => ({ heading, items }))
}
