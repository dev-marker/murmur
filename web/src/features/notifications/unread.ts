import { queryOptions } from '@tanstack/react-query'
import { request } from '../../lib/api'

// Read once; after that the stream counts up and reading counts down.
export const unreadOptions = queryOptions({
  queryKey: ['unread'],
  queryFn: () =>
    request<{ unread: number }>('/notifications/unread').then(
      (r) => r.unread,
    ),
  staleTime: Infinity,
})
