import { i18n } from '@/shared/i18n'

export type FeedbackTopicItem = {
  topic: string
  // Same-named constituents of different competencies have different IDs.
  topic_id?: string
  study_scope: 'review' | 'full' | 'skipped'
  text: string
}

const headings = {
  review: 'diagnostic.feedbackReviewTopic',
  full: 'diagnostic.feedbackStudyTopic',
  skipped: 'diagnostic.feedbackSkippedTopic',
} as const

// The backend writes one text per topic and repeats it on every finding of
// the topic: show each topic once, in the order of its first finding. Older
// reports without topic IDs are grouped by name.
export function groupFeedbackDetails(items: FeedbackTopicItem[]) {
  const groups = new Map<string, { heading: string; items: string[] }>()
  for (const item of items) {
    const text = item.text.trim()
    if (!text) continue
    const topic = item.topic.trim()
    const key = item.topic_id ? `id\0${item.topic_id}` : `name\0${topic}`
    let group = groups.get(key)
    if (!group) {
      group = {
        heading: topic
          ? i18n.t(headings[item.study_scope], { topic: topicName(topic) })
          : '',
        items: [],
      }
      groups.set(key, group)
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
