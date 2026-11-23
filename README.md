# Go System Design: Build a Social Network Backend

**Murmur**: a Twitter-style social network, built to learn system design by building it. A Go backend that is
measured, broken and fixed one wall at a time, and then a React web app on top of it. The code from the YouTube
course **Go System Design Full Course: Build a Social Network Backend From Scratch**. Every line was typed on
camera and every decision is explained in the videos.

- ▶ **The course on YouTube**: _link coming soon_
- ▶ **In chapters, with your progress saved**: _link coming soon_

## Follow along: one branch per episode

Each episode ends with a commit, and each episode has its own branch holding the code **as it is at the end of that
episode**. To start an episode where the video starts, check out the previous episode's branch:

```bash
git clone https://github.com/dev-marker/murmur.git
cd murmur
git checkout ep01   # the code at the end of episode 1, where episode 2 begins
```

`main` holds the latest episode plus this README.

The Go module is named `github.com/fiston-user/murmur`, as typed in the videos. It's only a name: everything builds
and runs from this clone as is.

| Branch | Episode | What we build |
|---|---|---|
| [`ep01`](https://github.com/dev-marker/murmur/tree/ep01) | 1. A Social Network in One Sitting | Users and posts over http, on postgres |
| [`ep02`](https://github.com/dev-marker/murmur/tree/ep02) | 2. Follows and the Home Feed | Follows and a home feed with cursor paging |
| [`ep03`](https://github.com/dev-marker/murmur/tree/ep03) | 3. The First Wall | A seed command, a k6 load test, and a feed query with a ceiling |
| [`ep04`](https://github.com/dev-marker/murmur/tree/ep04) | 4. Timelines in Redis | Home timelines in redis, with fan-out on write |
| [`ep05`](https://github.com/dev-marker/murmur/tree/ep05) | 5. The Celebrity Problem | Big accounts are pulled into the feed at read time |
| [`ep06`](https://github.com/dev-marker/murmur/tree/ep06) | 6. A Job Queue on Postgres | A job queue on postgres, and the fan-out leaves the request |
| [`ep07`](https://github.com/dev-marker/murmur/tree/ep07) | 7. When Jobs Fail | Retries with backoff, a dead list, and a sweeper for stuck jobs |
| [`ep08`](https://github.com/dev-marker/murmur/tree/ep08) | 8. Many Workers, and the Queue's Limit | Many workers, a clean shutdown, and numbers for the queue |
| [`ep09`](https://github.com/dev-marker/murmur/tree/ep09) | 9. A Rate Limiter from Scratch | A token bucket rate limiter, per user and per address |
| [`ep10`](https://github.com/dev-marker/murmur/tree/ep10) | 10. Two Servers, One Limit | One rate limit for every server, kept in redis |
| [`ep11`](https://github.com/dev-marker/murmur/tree/ep11) | 11. Likes and Notifications | Likes, counters that keep up, and notifications written by jobs |
| [`ep12`](https://github.com/dev-marker/murmur/tree/ep12) | 12. Live Updates | Live updates over server-sent events, a hub per server, and redis pub/sub |
| [`ep13`](https://github.com/dev-marker/murmur/tree/ep13) | 13. Measure Everything | Prometheus metrics, a grafana dashboard, and pprof |
| [`ep14`](https://github.com/dev-marker/murmur/tree/ep14) | 14. One Command to Run It All | One command to run everything, health checks, and restarts under load |
| [`ep15`](https://github.com/dev-marker/murmur/tree/ep15) | 15. The Big Load Test | A realistic load test, and the two fixes it asked for |
| [`ep16`](https://github.com/dev-marker/murmur/tree/ep16) | 16. Scaling Out, and What We Didn't Build | Scale out with one flag, and a read replica that knows who just wrote |
| [`ep17`](https://github.com/dev-marker/murmur/tree/ep17) | 17. What a Client Needs | Who a token is, a profile with counts, and who to follow |
| [`ep18`](https://github.com/dev-marker/murmur/tree/ep18) | 18. Lists That Know You | Lists that know the caller, a profile that pages, and a seed with words |
| [`ep19`](https://github.com/dev-marker/murmur/tree/ep19) | 19. A Shell That Routes | A Vite app with Tailwind, its tokens, the /api proxy, and a layout that routes |
| [`ep20`](https://github.com/dev-marker/murmur/tree/ep20) | 20. Signing In With a Key | A typed API client, the welcome page, loaders that guard routes |
| [`ep21`](https://github.com/dev-marker/murmur/tree/ep21) | 21. The Home Feed | The home feed, endless by cursor, and the composer |
| [`ep22`](https://github.com/dev-marker/murmur/tree/ep22) | 22. Likes, at Once | Likes at once, rolled back when the server says no |
| [`ep23`](https://github.com/dev-marker/murmur/tree/ep23) | 23. Profiles and Following | Profiles, the follow button, and who to follow |
| [`ep24`](https://github.com/dev-marker/murmur/tree/ep24) | 24. Notifications | Notifications, unread marks, and a badge on the bell |
| [`ep25`](https://github.com/dev-marker/murmur/tree/ep25) | 25. Live | The live stream: toasts, a live badge, and a new-posts pill |
| [`ep26`](https://github.com/dev-marker/murmur/tree/ep26) | 26. Ready for a Server | The built app behind nginx, and a calm 429 |

Episodes 1 to 16 are the backend. Episodes 17 and 18 add what a client needs from the API, and 19 to 26 build the
web app in [`web/`](web) and put it behind nginx.

## The system

| Piece | Where | What it does |
|---|---|---|
| API | http://localhost:8080 | two Go servers behind nginx: posts, follows, feeds, likes, notifications, a live stream |
| Web app | http://localhost:8088 | React and TypeScript, built by Vite into files that nginx serves, with the API under `/api/` |
| Worker | | the jobs behind a request, from a queue in Postgres |
| PostgreSQL 17 | localhost:5432 | a primary and a read replica |
| Redis 8 | localhost:6379 | timelines, the rate limiter, and the channel behind the live streams |
| Prometheus, Grafana | localhost:9090, localhost:3000 | metrics and a dashboard |

## Run it

You need Docker and `make`. Go 1.26 is needed for `make seed`, `make run` and `make test`, and Node.js with npm for
the web app (the course used Node 26).

```bash
make up && make seed   # the whole system, and 100,000 users with a realistic follow graph
make web               # build the web app into web/dist, served on http://localhost:8088
```

In that order: until `make web` has run, port 8088 answers 404. Sign in with a seeded key, `seed-token-N` for
`userN` (try `seed-token-16726`, a quiet account), or create an account.

```bash
make test              # the Go tests (needs make up)
make load S=feed       # a k6 load test, run in Docker
make web-dev           # the web app with hot reload, on http://localhost:5173
make down              # stop everything (the data stays in its volume)
```

The `Makefile` has a comment above every target, and [`web/README.md`](web/README.md) explains how the web app is
organised and why.

Older branches run with less: on the first episodes `make up` starts only Postgres and `make run` starts the API on
http://localhost:8081. Redis arrives in episode 4, the worker in episode 6, and from episode 14 everything runs in
Docker Compose behind nginx.

## License

MIT, see [LICENSE](LICENSE).
