import { useEffect, useRef, useState } from 'react'
import { createVoiceWaveform } from '@/shared/lib'
import styles from './trainer-layout.module.scss'

function QuietWaveform() {
  return <div className={styles.waveBars} />
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
