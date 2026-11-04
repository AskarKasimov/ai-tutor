// Keep the full time-domain window: at 48 kHz, 2048 samples cover about 43 ms.
// A short slice can miss a low-frequency voice peak depending on its phase.
export function monitorVoiceStream(
  stream: MediaStream,
  onPeaks: (peaks: Float32Array) => void,
) {
  const context = new AudioContext()
  let source: MediaStreamAudioSourceNode | undefined
  let analyser: AnalyserNode | undefined
  let timer: ReturnType<typeof setInterval> | undefined
  let disposed = false
  const stop = () => {
    if (disposed) return
    disposed = true
    if (timer !== undefined) clearInterval(timer)
    source?.disconnect()
    analyser?.disconnect()
    void context.close().catch(() => undefined)
  }

  try {
    source = context.createMediaStreamSource(stream)
    const meter = context.createAnalyser()
    analyser = meter
    meter.fftSize = 2048
    source.connect(meter)
    const samples = new Float32Array(meter.fftSize)
    const peaks = new Float32Array(300)
    timer = setInterval(() => {
      meter.getFloatTimeDomainData(samples)
      let peak = 0
      for (const value of samples) peak = Math.max(peak, Math.abs(value))
      peaks.copyWithin(0, 1)
      peaks[peaks.length - 1] = peak
      // WaveSurfer loads asynchronously; each render owns its snapshot.
      onPeaks(peaks.slice())
    }, 10)
    if (context.state === 'suspended')
      void context.resume().catch((error: unknown) => {
        if (!disposed)
          console.error('Error starting microphone visualization:', error)
      })
    return stop
  } catch (error) {
    stop()
    throw error
  }
}
