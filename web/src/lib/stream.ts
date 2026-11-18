import { sleep } from './api'

export type LiveEvent = { id: string; event: string; data: string }
export type Status = 'live' | 'away'

// parser turns text/event-stream into events. A chunk can end in the
// middle of an event, so what's left waits for the next chunk.
export function parser(onEvent: (e: LiveEvent) => void) {
  let rest = ''
  return (chunk: string) => {
    const blocks = (rest + chunk).split(/\r?\n\r?\n/)
    rest = blocks.pop() ?? ''
    for (const block of blocks) {
      const e = { id: '', event: 'message', data: '' }
      for (const line of block.split(/\r?\n/)) {
        // ": still here" is a comment, and doesn't match
        const [, field, value] = /^(\w+): ?(.*)$/.exec(line) ?? []
        if (field === 'data') e.data += (e.data && '\n') + value
        if (field === 'id' || field === 'event') e[field] = value
      }
      if (e.data) onEvent(e)
    }
  }
}

// listen holds GET /live open until signal aborts. EventSource can't
// send a token, so this is fetch and a reader. After a break it waits
// 1, 2, 4… up to 30 seconds, or what Retry-After says, and asks for
// what it missed with Last-Event-ID.
export async function listen(
  token: string,
  onEvent: (e: LiveEvent) => void,
  onStatus: (s: Status) => void,
  signal: AbortSignal,
) {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
  }
  const parse = parser((e) => {
    if (e.id) headers['Last-Event-ID'] = e.id
    onEvent(e)
  })
  let wait = 1000
  while (!signal.aborted) {
    try {
      const res = await fetch('/api/live', { headers, signal })
      const after = Number(res.headers.get('Retry-After')) * 1000
      wait = Math.max(wait, after)
      if (res.ok && res.body) {
        onStatus('live')
        wait = 1000
        const text = res.body.pipeThrough(new TextDecoderStream())
        const reader = text.getReader()
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          parse(value)
        }
      }
    } catch {
      // the network is gone, or we stopped: the loop decides
    }
    if (signal.aborted) return
    onStatus('away')
    await sleep(wait + Math.random() * 500)
    wait = Math.min(wait * 2, 30_000)
  }
}
