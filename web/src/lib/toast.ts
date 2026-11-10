// A tiny store for toasts. Anything may call toast(), and <Toasts>
// reads them with useSyncExternalStore.
type Toast = { id: number; text: string }

let toasts: Toast[] = []
let last = 0
const listeners = new Set<() => void>()

function set(next: Toast[]) {
  toasts = next
  listeners.forEach((listener) => listener())
}

export function toast(text: string) {
  if (toasts.some((t) => t.text === text)) return // once is enough
  const id = ++last
  set([...toasts, { id, text }].slice(-3))
  setTimeout(() => set(toasts.filter((t) => t.id !== id)), 4000)
}

export const getToasts = () => toasts
export function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
