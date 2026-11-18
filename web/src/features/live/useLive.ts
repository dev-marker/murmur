import { queryOptions, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { getSession } from '../../lib/api'
import { listen, type LiveEvent, type Status } from '../../lib/stream'
import { toast } from '../../lib/toast'
import { updatePost } from '../feed/posts'
import { unreadOptions } from '../notifications/unread'

// The posts the stream announced. Only the stream writes this;
// nothing fetches it.
export const freshOptions = queryOptions({
  queryKey: ['fresh'],
  queryFn: (): number[] => [],
  staleTime: Infinity,
})

type News = { actor: string; post_id: number }

// useLive keeps one stream open, and turns what it hears into changes
// to the cache. So nothing in the app has to poll.
export function useLive() {
  const client = useQueryClient()
  const [status, setStatus] = useState<Status>('away')
  useEffect(() => {
    const token = getSession()?.token
    if (!token) return

    function onEvent(e: LiveEvent) {
      const { actor, post_id: id }: News = JSON.parse(e.data)
      if (e.event === 'post') {
        client.setQueryData(freshOptions.queryKey, (ids = []) =>
          ids.includes(id) ? ids : [...ids, id],
        )
      }
      if (e.event === 'like') {
        toast(`@${actor} liked your post`)
        client.setQueryData(unreadOptions.queryKey, (n = 0) => n + 1)
        updatePost(client, id, (p) => ({ ...p, likes: p.likes + 1 }))
        client.invalidateQueries({ queryKey: ['notifications'] })
      }
    }

    const stop = new AbortController()
    listen(token, onEvent, setStatus, stop.signal)
    return () => stop.abort()
  }, [client])
  return status
}
