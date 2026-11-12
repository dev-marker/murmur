import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Avatar } from '../../components/ui/Avatar'
import { request } from '../../lib/api'
import { FollowButton } from './FollowButton'

type Suggestion = { handle: string; followers: number }

export function Suggestions() {
  const { data } = useQuery({
    queryKey: ['suggestions'],
    queryFn: () =>
      request<{ users: Suggestion[] }>('/suggestions').then(
        (r) => r.users,
      ),
  })
  if (!data?.length) return null
  return (
    <section aria-labelledby="suggest">
      <h2 id="suggest" className="text-sm font-semibold">
        Who to follow
      </h2>
      <ul className="mt-3 space-y-3">
        {data.map((s) => (
          <li key={s.handle} className="flex items-center gap-3">
            <Avatar handle={s.handle} size={32} />
            <div className="min-w-0 flex-1 text-sm">
              <Link
                to={`/u/${s.handle}`}
                className="block truncate font-medium hover:underline"
              >
                @{s.handle}
              </Link>
              <p className="text-mute">
                {s.followers.toLocaleString('en')} followers
              </p>
            </div>
            <FollowButton handle={s.handle} following={false} />
          </li>
        ))}
      </ul>
    </section>
  )
}
