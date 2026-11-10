import {
  infiniteQueryOptions,
  useMutation,
  type InfiniteData,
  type QueryClient,
} from '@tanstack/react-query'
import { request, type Page, type Post } from '../../lib/api'
import { toast } from '../../lib/toast'

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

// updatePost changes one post in every list that holds it.
export function updatePost(
  client: QueryClient,
  id: number,
  change: (post: Post) => Post,
) {
  client.setQueriesData<InfiniteData<Page>>(
    { queryKey: ['posts'] },
    (data) =>
      data && {
        ...data,
        pages: data.pages.map((page) => ({
          ...page,
          posts: page.posts.map((p) => (p.id === id ? change(p) : p)),
        })),
      },
  )
}

// useLike likes or unlikes at once, and asks the server afterwards.
// There's no refetch when it answers: the worker adds likes up once a
// second, and a replica may be older still, so a refetch would bring
// back the old count for a moment.
export function useLike() {
  return useMutation({
    mutationFn: (post: Post) =>
      request(`/posts/${post.id}/like`, {
        method: post.liked ? 'DELETE' : 'POST',
      }),
    onMutate: async (post, { client }) => {
      await client.cancelQueries({ queryKey: ['posts'] })
      updatePost(client, post.id, (p) => ({
        ...p,
        liked: !post.liked,
        likes: p.likes + (post.liked ? -1 : 1),
      }))
    },
    onError: (_error, post, _result, { client }) => {
      updatePost(client, post.id, () => post)
      toast("That like didn't go through.")
    },
  })
}
