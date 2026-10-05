import WaveSurfer from 'wavesurfer.js'
import RecordPlugin from 'wavesurfer.js/dist/plugins/record.esm.js'

export function createVoiceWaveform(container: HTMLElement) {
  const waveform = WaveSurfer.create({
    container,
    height: 82,
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
      if (!stream) { showSilence(); return }
      const plugin = waveform.registerPlugin(RecordPlugin.create({
        scrollingWaveform: true,
        scrollingWaveformWindow: 3,
        renderRecordedAudio: false,
      }))
      try {
        const monitor = plugin.renderMicStream(stream)
        stopMonitor = () => { monitor.onDestroy(); plugin.destroy() }
        activeStream = stream
      } catch (error) {
        plugin.destroy()
        throw error
      }
    },
    dispose() { stopMonitor?.(); stopMonitor = undefined; waveform.destroy() },
  }
}
