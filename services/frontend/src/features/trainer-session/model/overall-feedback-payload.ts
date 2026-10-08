import type { TrainerSession } from '@/entities/trainer-session'
import type { OverallFeedbackItemInput } from '@/entities/assessment'

const taskTopicMap: Record<string, string> = {
  ml_001: 'Классификация (типы задач ML)',
  ml_002: 'Регрессия (типы задач ML)',
  ml_014: 'Кластеризация (типы задач ML)',
  ml_015: 'Ранжирование (типы задач ML)',
  ml_016: 'Диагностика переобучения (Overfitting / Underfitting)',
  ml_017: 'Схема валидации данных (Train / Val / Test)',
  ml_018: 'Метрики качества при дисбалансе классов (Precision / Recall / F1)',
}

export function buildOverallFeedbackPayload(
  session: TrainerSession,
  translate: (key: string) => string,
): OverallFeedbackItemInput[] {
  return session.tasks.map((task) => {
    const answer = session.answers.find(
      (item) => item.assignmentId === task.assignmentId,
    )
    return {
      task_id: task.taskId,
      topic: taskTopicMap[task.taskId] ?? translate(task.questionKey),
      question: translate(task.questionKey),
      transcript: answer?.transcript ?? '',
      score: answer?.assessment.score ?? 0,
      max_score: 2,
      feedback: answer?.assessment.feedback ?? [],
    }
  })
}
