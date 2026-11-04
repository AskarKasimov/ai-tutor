import { Button, Flex, Heading, Progress, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import type { AudioPreparationDirection } from '@/entities/subject'
import { useAudioPreparationQuery } from '@/features/subject-selection'
import styles from './admin-course-detail-shell.module.scss'

type Props = {
  userId: string
  subjectId: string
  direction: 'learn' | 'exam'
  stale?: boolean
}

export function AudioPreparationPanel({
  userId,
  subjectId,
  direction,
  stale = false,
}: Props) {
  const { t } = useTranslation()
  const query = useAudioPreparationQuery(userId, subjectId)
  const audio = query.data?.[direction]
  const title =
    direction === 'exam'
      ? t('audioPreparation.examTitle')
      : t('audioPreparation.learnTitle')

  return (
    <section
      className={styles.audioCard}
      aria-labelledby="audio-preparation-title"
    >
      <Heading as="h3" size="4" id="audio-preparation-title">
        {title}
      </Heading>
      {query.isPending ? (
        <Text as="p" role="status" color="gray">
          {t('audioPreparation.loading')}
        </Text>
      ) : query.isError ? (
        <Flex gap="3" align="center" wrap="wrap">
          <Text as="p" role="status" color="red">
            {t('audioPreparation.error')}
          </Text>
          <Button
            variant="soft"
            size="1"
            aria-label={t('audioPreparation.retry')}
            onClick={() => void query.refetch()}
          >
            {t('trainer.retry')}
          </Button>
        </Flex>
      ) : audio ? (
        <AudioProgress
          direction={direction}
          audio={audio}
          title={title}
          stale={direction === 'exam' && stale}
        />
      ) : null}
    </section>
  )
}

function AudioProgress({
  direction,
  audio,
  title,
  stale,
}: {
  direction: 'learn' | 'exam'
  audio: AudioPreparationDirection & { published?: boolean }
  title: string
  stale: boolean
}) {
  const { t } = useTranslation()
  if (direction === 'exam' && audio.published === false)
    return (
      <Text as="p" role="status" color="gray">
        {t('audioPreparation.afterPublication')}
      </Text>
    )
  if (direction === 'learn' && 'revision' in audio && audio.revision === 0)
    return (
      <Text as="p" role="status" color="gray">
        {t('audioPreparation.afterMapImport')}
      </Text>
    )
  if (audio.total === 0)
    return (
      <Text as="p" role="status" color="gray">
        {t('audioPreparation.empty')}
      </Text>
    )

  const percent = Math.floor((audio.ready * 100) / audio.total)
  const counters = [
    ['queued', audio.queued],
    ['processing', audio.processing],
    ['retry_wait', audio.retry_wait],
    ['failed', audio.failed],
    ['unavailable', audio.unavailable],
    ['unchecked', audio.unchecked],
  ] as const
  return (
    <div className={styles.audioContent}>
      <Text as="p" weight="medium" role="status">
        {t(`audioPreparation.states.${audio.state}`)}
      </Text>
      <Text as="p" size="2">
        {t('audioPreparation.readyCount', {
          ready: audio.ready,
          total: audio.total,
          percent,
        })}
      </Text>
      <Progress
        value={percent}
        max={100}
        aria-label={t('audioPreparation.progressLabel', { title, percent })}
      />
      {stale && (
        <Text as="p" size="1" color="gray">
          {t('audioPreparation.publishedVersion')}
        </Text>
      )}
      <ul className={styles.audioCounters}>
        {counters
          .filter(([, count]) => count > 0)
          .map(([key, count]) => (
            <li key={key}>
              {t(`audioPreparation.counters.${key}`, { count })}
            </li>
          ))}
      </ul>
    </div>
  )
}
