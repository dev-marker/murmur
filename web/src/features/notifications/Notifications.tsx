import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { Heart } from 'lucide-react'
import { useEffect } from 'react'
import { Link } from 'react-router'
import { More } from '../../components/ui/More'
import { Failed, Notice, Skeletons } from '../../components/ui/Notice'
import { getSession, request, type Notes } from '../../lib/api'
import { ago } from '../../lib/time'
import { unreadOptions } from './unread'

const notesOptions = infiniteQueryOptions({
  queryKey: ['notifications'],
  queryFn: ({ pageParam }) =>
    request<Notes>(`/notifications?cursor=${pageParam || ''}`),
  initialPageParam: 0,
  getNextPageParam: (last) => last.next_cursor,
})

export function Notifications() {
  const client = useQueryClient()
  const query = useInfiniteQuery(notesOptions)
  const handle = getSession()?.handle
  const notes = query.data?.pages.flatMap((p) => p.notifications) ?? []
  const { mutate: read } = useMutation({
    mutationFn: (id: number) =>
      request('/notifications/read', {
        method: 'POST',
        json: { up_to: id },
      }),
    onSuccess: () => client.setQueryData(unreadOptions.queryKey, 0),
  })
  // Seen is read: the newest one marks everything up to it. The marks
  // stay on screen until the next visit, so you can tell what's new.
  const newest = notes[0]
  useEffect(() => {
    if (newest?.unread) read(newest.id)
  }, [newest, read])

  return (
    <>
      <h1 className="bar">Notifications</h1>
      {query.isPending ? (
        <Skeletons />
      ) : query.isError ? (
        <Failed error={query.error} retry={query.refetch} />
      ) : !notes.length ? (
        <Notice title="Nothing yet">
          When someone likes your post, it shows up here.
        </Notice>
      ) : (
        <>
          <ol className="divide-y divide-line border-b border-line">
            {notes.map((n) => (
              <li
                key={n.id}
                className={`flex gap-3 px-4 py-3 text-sm
                  ${n.unread ? 'bg-accent/8' : ''}`}
              >
                <Heart
                  size={16}
                  className="mt-0.5 shrink-0 text-accent-ink"
                />
                <div className="min-w-0 flex-1">
                  <p className="flex gap-2">
                    <span className="truncate">
                      <Link
                        to={`/u/${n.actor}`}
                        className="font-medium hover:underline"
                      >
                        @{n.actor}
                      </Link>{' '}
                      liked your post
                    </span>
                    <time
                      dateTime={n.created_at}
                      className="ml-auto text-mute"
                    >
                      {ago(n.created_at)}
                    </time>
                  </p>
                  <Link
                    to={`/u/${handle}`}
                    className="block truncate text-mute hover:text-ink"
                  >
                    {n.body}
                  </Link>
                </div>
                {n.unread && <span className="sr-only">(new)</span>}
              </li>
            ))}
          </ol>
          <More pages={query} />
        </>
      )}
    </>
  )
}
