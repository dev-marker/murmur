import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Notice } from '../../components/ui/Notice'
import { freshOptions } from '../live/useLive'
import { Suggestions } from '../profile/Suggestions'
import { Composer } from './Composer'
import { PostList } from './PostList'
import { postsOptions } from './posts'

const feedOptions = postsOptions('/feed', 'home')

export function Home() {
  const client = useQueryClient()
  const feed = useInfiniteQuery(feedOptions)
  const ids = useQuery(freshOptions).data ?? []
  const posts = feed.data?.pages.flatMap((p) => p.posts)
  // Counted against the feed on screen: a post it shows isn't news,
  // even when the stream announced it before the feed was read.
  const fresh = posts
    ? ids.filter((id) => !posts.some((p) => p.id === id)).length
    : 0

  // New posts wait behind a pill in the bar, so the page never moves
  // under you and nothing is covered.
  // Reset drops the pages read so far and fetches the first one again.
  function showFresh() {
    client.setQueryData(freshOptions.queryKey, [])
    client.resetQueries({ queryKey: feedOptions.queryKey })
    window.scrollTo({ top: 0 })
  }

  return (
    <>
      <header className="bar flex items-center justify-between">
        <h1>Home</h1>
        {fresh > 0 && (
          <button
            onClick={showFresh}
            className="btn py-0.5 motion-safe:animate-drop"
          >
            {fresh} new {fresh === 1 ? 'post' : 'posts'}
          </button>
        )}
      </header>
      <Composer />
      <PostList
        query={feed}
        empty={
          <Notice title="Your feed is quiet">
            Follow a few people and their posts land here.
            <div className="mt-6 text-left">
              <Suggestions />
            </div>
          </Notice>
        }
      />
    </>
  )
}
