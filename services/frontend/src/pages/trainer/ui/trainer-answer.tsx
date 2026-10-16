import { useEffect, useRef, useState } from 'react'
import { createVoiceWaveform } from '@/shared/lib'
import styles from './trainer-layout.module.scss'

function QuietWaveform() {
  return <div className={styles.waveBars} />
}

export function LiveWaveform({
  stream,
  audio,
  height,
  gain,
}: {
  stream?: MediaStream
  // A finished recording drawn instead of the live microphone.
  audio?: Blob
  height?: number
  // Vertical amplification of the live microphone bars.
  gain?: number
}) {
  const container = useRef<HTMLDivElement>(null)
  const waveform = useRef<ReturnType<typeof createVoiceWaveform> | null>(null)
  const [unavailable, setUnavailable] = useState(false)
  useEffect(() => {
    if (!container.current) return
    try {
      waveform.current = createVoiceWaveform(container.current, height, gain)
    } catch {
      setUnavailable(true)
    }
    return () => {
      waveform.current?.dispose()
      waveform.current = null
    }
    // The height and gain are fixed for the lifetime of the waveform.
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
    if (!audio) return
    try {
      waveform.current?.setAudio(audio)
    } catch {
      waveform.current?.dispose()
      waveform.current = null
      setUnavailable(true)
    }
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
