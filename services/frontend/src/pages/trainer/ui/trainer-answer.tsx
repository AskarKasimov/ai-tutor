import { useEffect, useRef, useState } from 'react'
import { Button, Flex } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { createVoiceWaveform } from '@/shared/lib'
import styles from './trainer-layout.module.scss'
import sessionStyles from './session.module.scss'

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
    // Both actions span the panel, so they share one size.
    <Flex direction="column" gap="2" width="100%">
      <Button
        size="3"
        className={sessionStyles.primary}
        disabled={disabled}
        onClick={onSend}
      >
        {t('trainer.sendAnswer')}
      </Button>
      <Button
        size="3"
        variant="soft"
        className={sessionStyles.primary}
        disabled={disabled}
        onClick={onRerecord}
      >
        {t('trainer.rerecordAnswer')}
      </Button>
    </Flex>
  )
}

export function LiveWaveform({
  stream,
  audio,
  height,
}: {
  stream?: MediaStream
  // A finished recording drawn instead of the live microphone.
  audio?: Blob
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
  useEffect(() => {
    if (audio) waveform.current?.setAudio(audio)
  }, [audio])
  return unavailable ? (
    <QuietWaveform />
  ) : (
    <div
      ref={container}
      className={styles.liveWaveform}
      data-recorded={audio ? 'true' : undefined}
    />
  )
}
