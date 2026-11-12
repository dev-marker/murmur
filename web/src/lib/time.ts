const format = new Intl.RelativeTimeFormat('en', { style: 'short' })
const units = [
  ['year', 31_536_000],
  ['month', 2_592_000],
  ['day', 86_400],
  ['hour', 3_600],
  ['minute', 60],
] as const

// ago says how long ago a time was: "3 hr. ago", "just now".
export function ago(iso: string) {
  const seconds = (Date.parse(iso) - Date.now()) / 1000
  for (const [unit, size] of units) {
    if (-seconds >= size) {
      return format.format(Math.round(seconds / size), unit)
    }
  }
  return 'just now'
}

export const month = (iso: string) =>
  new Date(iso).toLocaleDateString('en', {
    month: 'long',
    year: 'numeric',
  })
