import { useEffect, useRef, useState } from 'react'
import { Button, Flex, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { createVoiceWaveform } from '@/shared/lib'
import styles from './trainer-layout.module.scss'

function QuietWaveform() {
  return <div className={styles.waveBars} />
}

export function ConfirmVoiceAnswer({
  disabled,
  onSend,
  onRerecord,
}: {
  disabled?: boolean
  onSend: () => void
  onRerecord: () => void
}) {
  const { t } = useTranslation()
  return (
    <Flex direction="column" gap="3" width="100%">
      <Text size="2">{t('trainer.reviewTranscript')}</Text>
      <Flex gap="2" wrap="wrap" justify="between">
        <Button
          size="3"
          variant="soft"
          disabled={disabled}
          onClick={onRerecord}
        >
          {t('trainer.rerecordAnswer')}
        </Button>
        <Button size="3" disabled={disabled} onClick={onSend}>
          {t('trainer.sendAnswer')}
        </Button>
      </Flex>
    </Flex>
  )
}

export function LiveWaveform({
  stream,
  height,
}: {
  stream?: MediaStream
  height?: number
}) {
  const container = useRef<HTMLDivElement>(null)
  const waveform = useRef<ReturnType<typeof createVoiceWaveform> | null>(null)
  const [unavailable, setUnavailable] = useState(false)
  useEffect(() => {
    if (!container.current) return
    try {
      waveform.current = createVoiceWaveform(container.current, height)
    } catch {
      setUnavailable(true)
    }
    return () => {
      waveform.current?.dispose()
      waveform.current = null
    }
    // The height is fixed for the lifetime of the waveform.
  }, [])
  useEffect(() => {
    try {
      waveform.current?.setStream(stream)
    } catch {
      waveform.current?.dispose()
      waveform.current = null
      setUnavailable(true)
    }
  }, [stream])
  return unavailable ? (
    <QuietWaveform />
  ) : (
    <div ref={container} className={styles.liveWaveform} />
  )
}
