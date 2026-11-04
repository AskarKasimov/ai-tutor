import WaveSurfer from 'wavesurfer.js'
import { monitorVoiceStream } from './monitor-voice-stream'

// Gain scales live microphone bars so ordinary speech is clearly visible.
export function createVoiceWaveform(
  container: HTMLElement,
  height = 82,
  gain = 1,
) {
  const waveform = WaveSurfer.create({
    container,
    height,
    barHeight: gain,
    waveColor: '#6c99f2',
    cursorWidth: 0,
    barWidth: 5,
    barGap: 4,
    barRadius: 3,
    barMinHeight: 4,
    interact: false,
    hideScrollbar: true,
  })
  let activeStream: MediaStream | undefined
  let stopMonitor: (() => void) | undefined
  const silence = new Float32Array(300)
  function showSilence() {
    void waveform.load('', [silence], 3).catch((error: unknown) => {
      if (error instanceof DOMException && error.name === 'AbortError') return
      console.error('Error rendering silent waveform:', error)
    })
  }
  showSilence()
  return {
    setStream(stream?: MediaStream) {
      if (stream === activeStream) return
      stopMonitor?.()
      stopMonitor = undefined
      activeStream = undefined
      if (!stream) {
        showSilence()
        return
      }
      waveform.setOptions({ normalize: false, barHeight: gain })
      const stopStream = monitorVoiceStream(stream, (peaks) => {
        void waveform.load('', [peaks], 3).catch((error: unknown) => {
          if (error instanceof DOMException && error.name === 'AbortError')
            return
          console.error('Error rendering microphone waveform:', error)
        })
      })
      stopMonitor = () => {
        stopStream()
      }
      activeStream = stream
    },
    // Draws a finished recording; the silent line stays if it cannot be decoded.
    setAudio(audio?: Blob) {
      if (activeStream) return
      if (!audio) {
        showSilence()
        return
      }
      // A finished answer is stretched to the full height, however quiet.
      waveform.setOptions({ normalize: true, barHeight: 1 })
      void waveform.loadBlob(audio).catch(() => showSilence())
    },
    dispose() {
      stopMonitor?.()
      stopMonitor = undefined
      waveform.destroy()
    },
  }
}
