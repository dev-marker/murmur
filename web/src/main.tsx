import {
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router'
import './index.css'
import { ApiError, signOut } from './lib/api'
import { router } from './routes'

const client = new QueryClient({
  // A token the server doesn't know any more: start again.
  queryCache: new QueryCache({
    onError: (e) => {
      if (e instanceof ApiError && e.status === 401) signOut()
    },
  }),
  defaultOptions: {
    queries: {
      // 60 requests a minute is the budget. The stream says what
      // changed, so a read stays good for a minute, and coming back
      // to the tab is no reason to ask again.
      staleTime: 60_000,
      refetchOnWindowFocus: false,
      // A 4xx won't get better by asking again.
      retry: (n, e) =>
        n < 2 && !(e instanceof ApiError && e.status < 500),
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
