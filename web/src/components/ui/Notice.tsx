import type { ReactNode } from 'react'
import { Link } from 'react-router'

// Notice is what a screen says instead of a list: empty, failed, gone.
export function Notice(props: { title: string; children?: ReactNode }) {
  return (
    <div className="px-6 py-16 text-center">
      <p className="font-medium">{props.title}</p>
      <div className="mt-1 text-sm text-mute">{props.children}</div>
    </div>
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
