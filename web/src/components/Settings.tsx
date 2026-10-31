import { Moon, Sun } from 'lucide-react'

function toggleTheme() {
  const dark = document.documentElement.classList.toggle('dark')
  localStorage.theme = dark ? 'dark' : 'light'
}

type Props = { className: string; button: string; label?: string }

// The theme isn't a place, so it stays out of the phone's bottom bar.
export function Settings({ className, button, label }: Props) {
  return (
    <div className={className}>
      <button onClick={toggleTheme} className={button}>
        <Moon size={20} className="dark:hidden" aria-hidden />
        <Sun size={20} className="hidden dark:block" aria-hidden />
        <span className={label}>Theme</span>
      </button>
    </div>
  )
}
