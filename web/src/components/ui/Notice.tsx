import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { ApiError } from '../../lib/api'

// Notice is what a screen says instead of a list: empty, failed, gone.
export function Notice(props: { title: string; children?: ReactNode }) {
  return (
    <div className="px-6 py-16 text-center">
      <p className="font-medium">{props.title}</p>
      <div className="mt-1 text-sm text-mute">{props.children}</div>
    </div>
  )
}

export function Failed(props: { error: Error; retry: () => void }) {
  const offline = !(props.error instanceof ApiError)
  return (
    <Notice title={offline ? "Can't reach Murmur" : "That didn't load"}>
      <p className="first-letter:uppercase">
        {offline ? 'Check your connection.' : props.error.message}
      </p>
      <button className="btn-quiet mt-4" onClick={props.retry}>
        Try again
      </button>
    </Notice>
  )
}

export function NotFound() {
  return (
    <Notice title="Nothing here">
      <Link to="/" className="text-accent-ink underline">
        Back home
      </Link>
    </Notice>
  )
}

export function Skeletons() {
  return (
    <ul aria-label="Loading" className="motion-safe:animate-pulse">
      {[0, 1, 2, 3].map((i) => (
        <li key={i} className="flex gap-3 border-b border-line p-4">
          <div className="size-9 rounded-full bg-hover" />
          <div className="flex-1 space-y-2 py-1">
            <div className="h-3 w-1/4 rounded bg-hover" />
            <div className="h-3 w-3/4 rounded bg-hover" />
          </div>
        </li>
      ))}
    </ul>
  )
}
