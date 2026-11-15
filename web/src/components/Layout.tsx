import { useQuery } from '@tanstack/react-query'
import { Bell, House, User, type LucideIcon } from 'lucide-react'
import { NavLink, Outlet } from 'react-router'
import { unreadOptions } from '../features/notifications/unread'
import { Suggestions } from '../features/profile/Suggestions'
import { getSession } from '../lib/api'
import { Settings } from './Settings'
import { Toasts } from './ui/Toasts'

const item = `relative flex items-center gap-4 rounded-full p-3
  text-mute hover:bg-hover hover:text-ink lg:px-4`

export function Layout() {
  const handle = getSession()?.handle
  const unread = useQuery(unreadOptions).data ?? 0

  const link = (to: string, Icon: LucideIcon, label: string, n = 0) => (
    <NavLink
      to={to}
      end
      className={({ isActive }) =>
        isActive ? `${item} bg-hover font-semibold text-ink!` : item
      }
    >
      <Icon size={22} aria-hidden />
      <span className="sr-only lg:not-sr-only">{label}</span>
      {n > 0 && (
        <span
          className="absolute top-1.5 left-7 rounded-full bg-accent px-1.5
            text-xs font-semibold text-on-accent tabular-nums"
        >
          {n > 99 ? '99+' : n}
          <span className="sr-only"> unread</span>
        </span>
      )}
    </NavLink>
  )

  return (
    <div
      className="mx-auto max-w-6xl sm:grid sm:grid-cols-[5rem_1fr]
        lg:grid-cols-[15rem_minmax(0,38rem)_1fr]"
    >
      <nav
        aria-label="Main"
        className="fixed inset-x-0 bottom-0 z-20 flex justify-around
          border-t border-line bg-paper sm:sticky sm:top-0 sm:h-dvh
          sm:flex-col sm:justify-start sm:gap-1 sm:border-0 sm:p-3"
      >
        <p className="hidden px-4 py-3 text-xl font-semibold lg:block">
          murmur<span className="text-accent-ink">.</span>
        </p>
        {link('/', House, 'Home')}
        {link('/notifications', Bell, 'Notifications', unread)}
        {link(`/u/${handle}`, User, 'Profile')}
        <Settings
          className="mt-auto hidden flex-col gap-1 sm:flex"
          button={item}
          label="sr-only lg:not-sr-only"
        />
      </nav>

      <main className="min-h-dvh pb-16 sm:border-x sm:border-line sm:pb-0">
        <Outlet />
      </main>

      <aside className="sticky top-0 hidden h-dvh space-y-8 p-6 lg:block">
        <Suggestions />
      </aside>
      <Toasts />
    </div>
  )
}
