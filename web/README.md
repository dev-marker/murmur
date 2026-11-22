# Murmur on the web

A single-page app for the Murmur API: React and TypeScript, built by
Vite into static files that nginx serves next to the API.

## Run it

```sh
make up && make seed   # the whole system, and 100,000 users
make web               # build into web/dist, served on :8088
make web-dev           # or: http://localhost:5173, with hot reload
```

In that order: until `make web` has run, `web/dist` is empty and
port 8088 answers 404. A new build needs no restart of nginx.

Sign in with a seeded key, `seed-token-N` for `userN` (try 16726, a
quiet account, or 363, who follows 199), or create an account.

`npm test -- --run` runs the tests once, `npm run lint` and
`npm run build` check the rest, `npm run format` formats.

## How it is organised

```
src/
  main.tsx            the query client and the router
  routes.tsx          every route, in one place
  index.css           Tailwind, the design tokens, four small classes
  lib/                no React here
    api.ts            request(), the session, ApiError, response types
    stream.ts         reads GET /live: a parser, and reconnects
    toast.ts          a tiny store for toasts
    time.ts           "3 hr. ago"
  components/
    Layout.tsx        navigation, the right rail, the Live line
    Settings.tsx      theme and sign out
    ui/               Avatar, Notice, More, Toasts
  features/
    auth/             welcome, sign up, sign in
    feed/             home, composer, posts and likes
    profile/          profile, follow, who to follow
    notifications/    the list and the unread count
    live/             the stream, turned into cache updates
```

## Decisions, and why

- **One origin.** The app calls `/api/…`. Vite's dev server and, in
  production, nginx (`deploy/nginx.conf`, port 8088) pass it to the
  API. No CORS, and the API's own port 8080 stays as it was.
- **The stream is `fetch`, not `EventSource`**, because `EventSource`
  can't send the bearer token. `lib/stream.ts` parses the events,
  waits 1, 2, 4… up to 30 s after a break (or what `Retry-After`
  says), and sends `Last-Event-ID` to get what it missed.
- **The stream feeds the cache, so nothing polls.** A like moves the
  badge, the post's count and shows a toast. A post by someone you
  follow adds to an "N new posts" pill instead of moving the page.
- **The key and its handle are kept together**, so the shell never
  waits to know who you are, and starting costs one request less.
- **Rate limits are 60 requests a minute.** Reads stay fresh for a
  minute, nothing refetches on window focus, 4xx answers are not
  retried, and a 429 waits for `Retry-After` twice, saying so
  quietly; after that the screen shows the error.
- **Likes are optimistic and not refetched.** The worker adds likes up
  once a second and reads may come from a replica, so a refetch right
  after a like would show the old count. A failed like is put back.
- **Follow shows what you asked for while it's sent** (TanStack
  Query's "via the UI" way), then refetches: follower counts are
  written at once, and your own reads go to the primary.
- **Tokens, not `dark:`.** Colours are theme variables; dark mode
  gives them other values. Components use `bg-paper`, `text-mute`…
- **Long class lists** break over lines inside the string; a style
  used everywhere on one element is a class in `index.css` (`.btn`,
  `.field`, `.bar`); anything bigger is a component, as Tailwind's
  docs suggest.
- **No avatars from the internet**: `Avatar` draws a pattern from the
  handle. The font is Inter, bundled by Fontsource.
