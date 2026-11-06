import type {
  InfiniteData,
  UseInfiniteQueryResult,
} from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { More } from '../../components/ui/More'
import { Failed, Skeletons } from '../../components/ui/Notice'
import type { Page } from '../../lib/api'
import { PostItem } from './PostItem'

type Props = {
  query: UseInfiniteQueryResult<InfiniteData<Page>>
  empty: ReactNode
}

export function PostList({ query, empty }: Props) {
  if (query.isPending) return <Skeletons />
  if (query.isError) {
    return <Failed error={query.error} retry={query.refetch} />
  }
  const posts = query.data.pages.flatMap((page) => page.posts)
  if (!posts.length) return empty
  return (
    <>
      <ol className="divide-y divide-line border-b border-line">
        {posts.map((post) => (
          <li key={post.id}>
            <PostItem post={post} />
          </li>
        ))}
      </ol>
      <More pages={query} />
    </>
  )
}
