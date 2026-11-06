// What the API answers with.
export type Me = { id: number; handle: string }
export type Post = {
  id: number
  handle: string
  body: string
  created_at: string
  likes: number
  liked: boolean
}
export type Page = { posts: Post[]; next_cursor: number | null }

// The key and whose it is, kept together: the app never has to ask.
type Session = { token: string; handle: string }
const key = 'murmur.session'
export function getSession(): Session | null {
  return JSON.parse(localStorage.getItem(key) ?? 'null')
}
export const setSession = (session: Session) =>
  localStorage.setItem(key, JSON.stringify(session))
export function signOut() {
  localStorage.removeItem(key)
  location.assign('/welcome') // a fresh start: no cache, no stream
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

// request calls the API under /api, the app's own address, with the
// token. It turns {"error": "…"} into an ApiError.
export async function request<T>(
  path: string,
  { json, ...init }: RequestInit & { json?: unknown } = {},
): Promise<T> {
  const headers = new Headers(init.headers)
  const token = getSession()?.token
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (json !== undefined) {
    headers.set('Content-Type', 'application/json')
    init.body = JSON.stringify(json)
  }
  const res = await fetch(`/api${path}`, { ...init, headers })
  if (!res.ok) {
    const { error } = await res.json().catch(() => ({}))
    throw new ApiError(res.status, error ?? res.statusText)
  }
  return res.status === 204 ? (undefined as T) : res.json()
}
