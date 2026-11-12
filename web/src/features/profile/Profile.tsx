import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { Avatar } from '../../components/ui/Avatar'
import { Failed, Notice, NotFound } from '../../components/ui/Notice'
import {
  ApiError,
  getSession,
  request,
  type Profile as P,
} from '../../lib/api'
import { month } from '../../lib/time'
import { PostList } from '../feed/PostList'
import { postsOptions } from '../feed/posts'
import { Settings } from '../../components/Settings'
import { FollowButton } from './FollowButton'

export function Profile() {
  const { handle = '' } = useParams()
  const mine = getSession()?.handle === handle
  const user = useQuery({
    queryKey: ['user', handle],
    queryFn: () => request<P>(`/users/${handle}`),
  })
  const posts = useInfiniteQuery(
    postsOptions(`/users/${handle}/posts`, `@${handle}`),
  )
  if (user.error instanceof ApiError && user.error.status === 404) {
    return <NotFound />
  }
  if (user.isError) {
    return <Failed error={user.error} retry={user.refetch} />
  }
  const p = user.data
  const counts = p && [
    [p.posts, 'posts'],
    [p.followers, 'followers'],
    [p.following, 'following'],
  ]

  return (
    <>
      <h1 className="bar">Profile</h1>
      <header className="border-b border-line px-4 py-6">
        <div className="flex items-center gap-4">
          <Avatar handle={handle} size={64} />
          <div className="min-w-0 flex-1">
            <p className="truncate text-xl font-semibold">@{handle}</p>
            <p className="h-5 text-sm text-mute">
              {p && `Joined ${month(p.joined)}`}
            </p>
          </div>
          {p && !mine && (
            <FollowButton
              handle={handle}
              following={p.followed_by_me}
            />
          )}
        </div>
        <ul className="mt-5 flex h-5 gap-5 text-sm text-mute">
          {counts?.map(([n, label]) => (
            <li key={label}>
              <b className="font-semibold text-ink tabular-nums">
                {n.toLocaleString('en')}
              </b>{' '}
              {label}
            </li>
          ))}
        </ul>
        {mine && (
          <Settings
            className="mt-5 flex gap-2 sm:hidden"
            button="btn-quiet flex items-center gap-2"
          />
        )}
      </header>
      <PostList query={posts} empty={<Notice title="No posts yet" />} />
    </>
  )
}
