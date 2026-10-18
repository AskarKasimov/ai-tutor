#!/usr/bin/env node
// Synthetic PCM smoke in a real browser; no models, backend, cookies or mic.
// node scripts/smoke_voxcpm_browser.cjs <Vite URL> <playwright module> <Chrome executable>
const assert = require('node:assert/strict')
const fs = require('node:fs')

async function main() {
  const [url, playwrightPath, executablePath, outputPath] = process.argv.slice(2)
  if (!url || !playwrightPath || !executablePath) throw new Error('Expected Vite URL, Playwright module path and Chrome executable')
  const { chromium } = require(playwrightPath)
  const browser = await chromium.launch({ executablePath, headless: true })
  try {
    const page = await browser.newPage()
    // JS resource has the same localhost origin without booting the product UI.
    await page.goto(new URL('/@vite/client', url).href)
    const version = browser.version()
    await page.evaluate(async () => {
      const { playPcmStream } = await import('/src/shared/lib/pcm-player.ts')
      document.body.innerHTML = '<button id="play">Synthetic playback test</button>'
      window.runSmoke = (scenario) => new Promise((resolve, reject) => {
        document.querySelector('#play').onclick = async () => {
          const controller = new AbortController()
          const encoder = new TextEncoder()
          let closed = false
          let streamController
          let firstStartedBeforeEnd = false
          let starts = 0
          let cancelSeen = false
          const sourceRate = 16000
          const count = scenario === 'short' ? 800 : 8000
          const pcm = new Uint8Array(count * 2)
          const view = new DataView(pcm.buffer)
          for (let i = 0; i < count; i++) view.setInt16(i * 2, Math.round(Math.sin(i / sourceRate * 440 * Math.PI * 2) * 2000), true)
          const data = btoa(String.fromCharCode(...pcm))
          const event = value => streamController.enqueue(encoder.encode(JSON.stringify(value) + '\n'))
          const finish = () => {
            if (closed) return
            event({ type: 'end', chunks: 1, samples: count })
            streamController.close()
            closed = true
          }
          const body = new ReadableStream({
            start(c) {
              streamController = c
              event({ type: 'start', version: 1, sample_rate: sourceRate, channels: 1, encoding: 'pcm_s16le' })
              event({ type: 'audio', seq: 0, data })
              if (scenario === 'short') finish()
            },
            cancel() { cancelSeen = true; closed = true },
          })
          try {
            const result = await playPcmStream(body, controller.signal, () => {
              starts++
              firstStartedBeforeEnd = !closed
              if (scenario === 'cancel') controller.abort(new DOMException('Stopped', 'AbortError'))
              else finish()
            })
            resolve({ scenario, starts, firstStartedBeforeEnd, bytes: result.audio.size, metrics: result.metrics })
          } catch (error) {
            if (scenario !== 'cancel') reject(error)
            else resolve({ scenario, starts, cancelled: error.name === 'AbortError', cancelSeen })
          }
        }
      })
    })
    const results = []
    for (const scenario of ['early', 'short', 'cancel']) {
      const resultPromise = page.evaluate(scenario => window.runSmoke(scenario), scenario)
      await page.click('#play')
      let timer
      const result = await Promise.race([resultPromise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Playback timeout')), 10000) })])
        .finally(() => clearTimeout(timer))
      assert.equal(result.starts, 1)
      if (scenario === 'early') assert.equal(result.firstStartedBeforeEnd, true)
      if (scenario === 'short') assert.equal(result.bytes, 1644)
      if (scenario === 'cancel') { assert.equal(result.cancelled, true); assert.equal(result.cancelSeen, true) }
      results.push(result)
    }
    const report = { browser: version, synthetic: true, results,
      note: 'Real AudioWorklet and 16 kHz input on device AudioContext rate. Does not validate voice quality, physical audibility or Firefox/Safari.' }
    if (outputPath) fs.writeFileSync(outputPath, JSON.stringify(report, null, 2) + '\n')
    console.log(JSON.stringify(report, null, 2))
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
