import {
  QueryClient,
  QueryClientProvider,
  useInfiniteQuery,
  type InfiniteData,
} from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Page, Post } from '../../lib/api'
import { postsOptions, useLike } from './posts'

afterEach(() => vi.unstubAllGlobals())

const wrap =
  (client: QueryClient) =>
  ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      {children}
    </QueryClientProvider>
  )

const post = (id: number, likes = 0): Post => ({
  id,
  likes,
  liked: false,
  handle: 'alice',
  body: 'hello',
  created_at: '2026-01-01T00:00:00Z',
})

test('pages follow next_cursor until it is null', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({ posts: [post(9), post(8)], next_cursor: 8 }),
    )
    .mockResolvedValueOnce(
      Response.json({ posts: [post(7)], next_cursor: null }),
    )
  vi.stubGlobal('fetch', fetch)
  const { result } = renderHook(
    () => useInfiniteQuery(postsOptions('/feed', 'home')),
    { wrapper: wrap(new QueryClient()) },
  )
  await waitFor(() => expect(result.current.hasNextPage).toBe(true))
  await act(() => result.current.fetchNextPage())

  expect(fetch.mock.calls.map((c) => c[0])).toEqual([
    '/api/feed?cursor=',
    '/api/feed?cursor=8',
  ])
  await waitFor(() => expect(result.current.hasNextPage).toBe(false))
  const pages = result.current.data?.pages
  expect(pages?.flatMap((p) => p.posts.map((x) => x.id))).toEqual([
    9, 8, 7,
  ])
})

// The same post in the feed and on a profile: a like changes both at
// once, and a failed like puts both back.
test.each([
  ['it works', 204, { liked: true, likes: 4 }],
  ['it fails', 500, { liked: false, likes: 3 }],
])('a like shows at once, and when %s', async (_name, status, want) => {
  let answer: (r: Response) => void = () => {}
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise<Response>((r) => (answer = r))),
  )
  const client = new QueryClient()
  const page = {
    pages: [{ posts: [post(1, 3)], next_cursor: null }],
    pageParams: [0],
  }
  client.setQueryData(['posts', 'home'], page)
  client.setQueryData(['posts', '@alice'], page)
  const { result } = renderHook(useLike, { wrapper: wrap(client) })
  const seen = (key: string) =>
    client.getQueryData<InfiniteData<Page>>(['posts', key])?.pages[0]
      .posts[0]

  act(() => result.current.mutate(post(1, 3)))
  await waitFor(() => expect(seen('home')?.liked).toBe(true))
  expect(seen('@alice')).toMatchObject({ liked: true, likes: 4 })

  answer(new Response(status === 204 ? null : '{}', { status }))
  await waitFor(() => expect(result.current.isIdle).toBe(false))
  await waitFor(() => expect(result.current.isPending).toBe(false))
  expect(seen('home')).toMatchObject(want)
  expect(seen('@alice')).toMatchObject(want)
})
