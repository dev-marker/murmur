// Avatar draws a mirrored 5×5 pattern from the handle: every account
// gets its own, and nothing is loaded from anywhere.
export function Avatar({
  handle,
  size = 36,
}: {
  handle: string
  size?: number
}) {
  let hash = 2166136261
  for (const c of handle) {
    hash = Math.imul(hash ^ c.charCodeAt(0), 16777619) >>> 0
  }
  const color = `oklch(0.62 0.11 ${hash % 360})`
  const cells = []
  for (let i = 0; i < 15; i++) {
    if ((hash >>> (i + 8)) & 1 || i === 7) {
      const x = i % 3
      const y = Math.floor(i / 3)
      cells.push(<rect key={i} x={x} y={y} width="1" height="1" />)
      if (x < 2) {
        cells.push(
          <rect key={`m${i}`} x={4 - x} y={y} width="1" height="1" />,
        )
      }
    }
  }
  return (
    <svg
      viewBox="-1.5 -1.5 8 8"
      width={size}
      height={size}
      fill={color}
      className="shrink-0 rounded-full"
      style={{ background: `oklch(0.62 0.11 ${hash % 360} / 0.14)` }}
      aria-hidden
    >
      {cells}
    </svg>
  )
}
