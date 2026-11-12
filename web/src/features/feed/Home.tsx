import { useInfiniteQuery } from '@tanstack/react-query'
import { Notice } from '../../components/ui/Notice'
import { Suggestions } from '../profile/Suggestions'
import { Composer } from './Composer'
import { PostList } from './PostList'
import { postsOptions } from './posts'

const feedOptions = postsOptions('/feed', 'home')

export function Home() {
  const feed = useInfiniteQuery(feedOptions)

  return (
    <>
      <header className="bar flex items-center justify-between">
        <h1>Home</h1>
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
