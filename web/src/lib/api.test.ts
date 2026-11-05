import { afterEach, expect, test, vi } from 'vitest'
import { ApiError, request, setSession } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
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
