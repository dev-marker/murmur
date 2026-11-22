import { afterEach, expect, test, vi } from 'vitest'
import { ApiError, request, setSession } from './api'
import { getToasts } from './toast'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
  localStorage.clear()
})

test('request sends the token and JSON, and reads JSON', async () => {
  setSession({ token: 'abc', handle: 'alice' })
  const fetch = vi.fn().mockResolvedValue(Response.json({ id: 1 }))
  vi.stubGlobal('fetch', fetch)

  expect(
    await request('/posts', { method: 'POST', json: { body: 'hi' } }),
  ).toEqual({ id: 1 })
  const [url, init] = fetch.mock.calls[0]
  expect(url).toBe('/api/posts')
  expect(init.headers.get('Authorization')).toBe('Bearer abc')
  expect(init.body).toBe('{"body":"hi"}')
})

test('an error is an ApiError with what the server said', async () => {
  const taken = Response.json(
    { error: 'that handle is taken' },
    {
      status: 409,
    },
  )
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(taken))

  const error: ApiError = await request<never>('/users').catch((e) => e)
  expect(error).toBeInstanceOf(ApiError)
  expect([error.status, error.message]).toEqual([
    409,
    'that handle is taken',
  ])
})

test('a 429 waits for Retry-After, says so, and goes again', async () => {
  vi.useFakeTimers()
  const busy = new Response(null, {
    status: 429,
    headers: { 'Retry-After': '3' },
  })
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(busy)
    .mockResolvedValue(new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', fetch)

  const done = request('/posts/1/like', { method: 'POST' })
  await vi.advanceTimersByTimeAsync(2900)
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(getToasts().map((t) => t.text)).toEqual([
    'Busy moment. Trying again in 3s.',
  ])
  await vi.advanceTimersByTimeAsync(100)
  await done
  expect(fetch).toHaveBeenCalledTimes(2)
})

test('after two waits, a 429 is an error the screen shows', async () => {
  vi.useFakeTimers()
  const busy = () => new Response(null, { status: 429 })
  const fetch = vi.fn(async () => busy())
  vi.stubGlobal('fetch', fetch)

  const done = request<never>('/feed').catch((e: ApiError) => e.status)
  await vi.advanceTimersByTimeAsync(2000) // two waits of a second
  expect(await done).toBe(429)
  expect(fetch).toHaveBeenCalledTimes(3)
})
