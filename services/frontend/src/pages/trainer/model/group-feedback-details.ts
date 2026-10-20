import { i18n } from '@/shared/i18n'

export type FeedbackTopicItem = {
  topic: string
  study_scope: 'review' | 'full' | 'skipped'
  text: string
}

const headings = {
  review: 'diagnostic.feedbackReviewTopic',
  full: 'diagnostic.feedbackStudyTopic',
  skipped: 'diagnostic.feedbackSkippedTopic',
} as const

// The backend writes one text per topic and repeats it on every finding of
// the topic: show each topic once, in the order of its first finding.
export function groupFeedbackDetails(items: FeedbackTopicItem[]) {
  const groups = new Map<string, { heading: string; items: string[] }>()
  for (const item of items) {
    const text = item.text.trim()
    if (!text) continue
    const topic = item.topic.trim()
    let group = groups.get(topic)
    if (!group) {
      group = {
        heading: topic
          ? i18n.t(headings[item.study_scope], { topic: topicName(topic) })
          : '',
        items: [],
      }
      groups.set(topic, group)
    }
    if (!group.items.includes(text)) group.items.push(text)
  }
  return [...groups.values()]
}

// Map numbers such as «1.3» prefix constituent names; meaningful leading
// numbers like «2 типа обучения» stay.
function topicName(topic: string) {
  return topic.replace(/^\d+(?:\.\d+)+[.)]?\s+/, '').replace(/^Умеет\s+/i, '')
}
