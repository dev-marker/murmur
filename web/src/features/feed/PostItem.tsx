import { Heart } from 'lucide-react'
import { Link } from 'react-router'
import { Avatar } from '../../components/ui/Avatar'
import type { Post } from '../../lib/api'
import { ago } from '../../lib/time'

export function PostItem({ post }: { post: Post }) {
  return (
    <article className="flex gap-3 px-4 pt-3.5 pb-2">
      <Avatar handle={post.handle} />
      <div className="min-w-0 flex-1">
        <p className="flex gap-2 text-sm">
          <Link
            to={`/u/${post.handle}`}
            className="truncate font-medium hover:underline"
          >
            @{post.handle}
          </Link>
          <time dateTime={post.created_at} className="text-mute">
            {ago(post.created_at)}
          </time>
        </p>
        <p className="mt-0.5 whitespace-pre-wrap wrap-anywhere">
          {post.body}
        </p>
        <button
          aria-pressed={post.liked}
          aria-label={`Like, ${post.likes} so far`}
          className={`-ml-2 flex items-center gap-1.5 rounded-full
            px-2 py-1 text-sm tabular-nums hover:bg-hover
            ${post.liked ? 'text-accent-ink' : 'text-mute'}`}
        >
          <Heart
            size={16}
            fill={post.liked ? 'currentColor' : 'none'}
          />
          {post.likes}
        </button>
      </div>
    </article>
  )
}
