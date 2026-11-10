import { useSyncExternalStore } from 'react'
import { getToasts, subscribe } from '../../lib/toast'

export function Toasts() {
  const toasts = useSyncExternalStore(subscribe, getToasts)
  return (
    <div
      role="status"
      className="fixed inset-x-0 bottom-20 z-30 flex flex-col items-center
        gap-2 px-4 sm:bottom-6"
    >
      {toasts.map((t) => (
        <p
          key={t.id}
          className="rounded-full bg-ink px-4 py-2 text-sm text-paper
            shadow-sm motion-safe:animate-drop"
        >
          {t.text}
        </p>
      ))}
    </div>
  )
}
