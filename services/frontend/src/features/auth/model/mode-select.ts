// Start page that chooses between learning and the exam simulation
// (deploy/dual). Empty outside that stand: no link back, logout stays here.
export function modeSelectUrl() {
  return (import.meta.env.VITE_MODE_SELECT_URL || '').replace(/\/+$/, '')
}

// A full page load: the start page is another site.
export function openModeSelect(path = '') {
  window.location.assign(`${modeSelectUrl()}/${path}`)
}
