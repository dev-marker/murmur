import { infiniteQueryOptions } from '@tanstack/react-query'
import { request, type Page } from '../../lib/api'

// A list of posts, a page at a time: the home feed is ['posts', 'home'],
// a profile ['posts', '@user1']. The cursor is the last post's id.
export const postsOptions = (path: string, name: string) =>
  infiniteQueryOptions({
    queryKey: ['posts', name],
    queryFn: ({ pageParam }) =>
      request<Page>(`${path}?cursor=${pageParam || ''}`),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_cursor,
  })
