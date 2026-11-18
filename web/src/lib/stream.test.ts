import { afterEach, expect, test, vi } from 'vitest'
import { listen, parser, type LiveEvent } from './stream'

test.each([
  [
    'one event',
    ['id: 7\nevent: like\ndata: {"a":1}\n\n'],
    ['7 like {"a":1}'],
  ],
  [
    'split anywhere',
    ['id: 7\nevent: li', 'ke\ndata: {"a"', ':1}\n', '\n'],
    ['7 like {"a":1}'],
  ],
  ['comments are not events', [': connected\n\n: still here\n\n'], []],
  ['no id', ['event: post\ndata: {}\n\n'], [' post {}']],
  [
    'two at once',
    ['data: a\n\ndata: b\n\n'],
    [' message a', ' message b'],
  ],
  ['CRLF', ['event: post\r\ndata: x\r\n\r\n'], [' post x']],
  ['not finished yet', ['data: a\n'], []],
])('parser: %s', (_name, chunks, want) => {
  const got: string[] = []
  const parse = parser((e) => got.push(`${e.id} ${e.event} ${e.data}`))
  chunks.forEach(parse)
  expect(got).toEqual(want)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// A response whose body is these chunks, and then the end.
const stream = (...chunks: string[]) =>
  new Response(
    new ReadableStream({
      start(c) {
        chunks.forEach((s) => c.enqueue(new TextEncoder().encode(s)))
        c.close()
      },
    }),
  )

test('listen comes back after a break, with Last-Event-ID', async () => {
  vi.useFakeTimers()
  const answers = [
    stream('id: 41\nevent: like\ndata: {}\n\n'),
    new Response(null, { status: 502 }),
    stream('id: 42\nevent: like\ndata: {}\n\n'),
  ]
  const sent: (string | undefined)[] = []
  const fetch = vi.fn(async (_url: string, init: RequestInit) => {
    sent.push((init.headers as Record<string, string>)['Last-Event-ID'])
    return answers.shift() ?? stream()
  })
  vi.stubGlobal('fetch', fetch)
  const events: LiveEvent[] = []
  const statuses: string[] = []
  const stop = new AbortController()

  listen(
    't',
    (e) => events.push(e),
    (s) => statuses.push(s),
    stop.signal,
  )
  await vi.advanceTimersByTimeAsync(1500) // the first wait: 1 s
  await vi.advanceTimersByTimeAsync(2500) // after a failure: 2 s
  stop.abort()

  expect(events.map((e) => e.id)).toEqual(['41', '42'])
  expect(statuses).toEqual(['live', 'away', 'away', 'live', 'away'])
  expect(sent.slice(0, 3)).toEqual([undefined, '41', '41'])
})
