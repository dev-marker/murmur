type Pages = {
  hasNextPage: boolean
  isFetchingNextPage: boolean
  isFetchNextPageError: boolean
  fetchNextPage: () => unknown
}

// More fetches the next page when it scrolls into view, and it is a
// button for anyone who'd rather press it.
export function More({ pages }: { pages: Pages }) {
  if (!pages.hasNextPage) {
    return (
      <p className="py-10 text-center text-sm text-mute">That's all.</p>
    )
  }
  const idle = !pages.isFetchingNextPage && !pages.isFetchNextPageError
  return (
    <button
      ref={(el) => {
        if (!el || !idle) return
        const io = new IntersectionObserver(([entry]) => {
          if (entry.isIntersecting) pages.fetchNextPage()
        })
        io.observe(el)
        return () => io.disconnect()
      }}
      onClick={() => pages.fetchNextPage()}
      className="w-full py-6 text-sm text-mute hover:text-ink"
    >
      {pages.isFetchingNextPage ? 'Loading…' : 'Load more'}
    </button>
  )
}
