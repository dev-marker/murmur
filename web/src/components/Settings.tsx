import { LogOut, Moon, Sun } from 'lucide-react'
import { signOut } from '../lib/api'

function toggleTheme() {
  const dark = document.documentElement.classList.toggle('dark')
  localStorage.theme = dark ? 'dark' : 'light'
}

type Props = { className: string; button: string; label?: string }

// Theme and sign out. They aren't places, so they stay out of the
// phone's bottom bar: there they live on your own profile.
export function Settings({ className, button, label }: Props) {
  return (
    <div className={className}>
      <button onClick={toggleTheme} className={button}>
        <Moon size={20} className="dark:hidden" aria-hidden />
        <Sun size={20} className="hidden dark:block" aria-hidden />
        <span className={label}>Theme</span>
      </button>
      <button onClick={signOut} className={button}>
        <LogOut size={20} aria-hidden />
        <span className={label}>Sign out</span>
      </button>
    </div>
  )
}
