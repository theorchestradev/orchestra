export const preview = new URLSearchParams(location.search).get('preview') === '1'
let previewBackend
const listeners = new Map()
export function on(event, handler) {
  if (preview) {
    let set = listeners.get(event)
    if (!set) listeners.set(event, (set = new Set()))
    set.add(handler)
    return () => set.delete(handler)
  }
  return window.runtime?.EventsOn(event, handler) || (() => {})
}
// Wails accepts one file drop callback, so every terminal shares it and checks the drop point.
// It stays registered because its listeners also keep stray drops from opening the file.
const dropHandlers = new Set()
let dropRegistered = false
export function onFileDrop(handler) {
  if (preview || !window.runtime?.OnFileDrop) return () => {}
  if (!dropRegistered) window.runtime.OnFileDrop((x, y, paths) => dropHandlers.forEach((fn) => fn(x, y, paths)), false)
  dropRegistered = true
  dropHandlers.add(handler)
  return () => dropHandlers.delete(handler)
}
export function emitPreview(event, value) {
  listeners.get(event)?.forEach((fn) => fn(value))
}
export async function call(method, ...args) {
  if (preview) {
    previewBackend ||= import('./preview.js').then((m) => m.backend)
    return (await previewBackend)[method](...args)
  }
  const fn = window.go?.main?.App?.[method]
  if (!fn)
    throw Error('Open Orchestra through Wails to use native features. For the isolated visual preview, use ?preview=1.')
  return fn(...args)
}
export async function copy(text) {
  if (preview) return navigator.clipboard.writeText(text)
  return window.runtime.ClipboardSetText(text)
}
export async function paste() {
  if (preview) return navigator.clipboard.readText()
  return window.runtime.ClipboardGetText()
}
export function openURL(url) {
  if (!/^https?:\/\//i.test(url)) return
  if (preview) window.open(url, '_blank', 'noopener,noreferrer')
  else window.runtime.BrowserOpenURL(url)
}
