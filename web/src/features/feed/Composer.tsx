import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { getSession, request } from '../../lib/api'

// The server's limit. posts.Create counts characters, not bytes, and
// so does [...body].
const max = 280

export function Composer() {
  const [body, setBody] = useState('')
  const client = useQueryClient()
  const me = getSession()
  const post = useMutation({
    mutationFn: () =>
      request('/posts', { method: 'POST', json: { body } }),
    onSuccess: () => {
      setBody('')
      client.invalidateQueries({
        queryKey: ['posts', `@${me?.handle}`],
      })
    },
  })
  const left = max - [...body].length

  return (
    <form
      action={() => post.mutate()}
      className="border-b border-line px-4 py-3"
    >
      <label htmlFor="body" className="sr-only">
        New post
      </label>
      <textarea
        id="body"
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder="Say something, quietly."
        className="field field-sizing-content min-h-16 resize-none
          border-0 px-0 text-base outline-offset-4"
      />
      <div className="flex items-center justify-end gap-3 text-sm">
        {post.isError && (
          <p role="alert" className="mr-auto text-accent-ink">
            {post.error.message}
          </p>
        )}
        <span className={left < 0 ? 'text-accent-ink' : 'text-mute'}>
          {left}
        </span>
        <button
          className="btn"
          disabled={!body.trim() || left < 0 || post.isPending}
        >
          Post
        </button>
      </div>
    </form>
  )
}
