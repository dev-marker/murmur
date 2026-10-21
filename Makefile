# Local development. The API reads DATABASE_URL and REDIS_URL from the environment.
DATABASE_URL ?= postgres://murmur:murmur@localhost:5432/murmur?sslmode=disable&connect_timeout=5
REDIS_URL ?= redis://localhost:6379/0
# The proxy that a server run by hand believes (make run). The containers get their settings
# from docker-compose.yml, and they believe the Compose network.
TRUSTED_PROXIES ?= 127.0.0.1/32
export DATABASE_URL REDIS_URL TRUSTED_PROXIES

.PHONY: up down run test seed load explain redis worker jobs claim dead retry poison ps roll

# build our image and start everything: Postgres, Redis, two API servers, the worker, nginx,
# Prometheus (localhost:9090) and Grafana (localhost:3000). For load tests: make up IP_PER_MINUTE=6000000
up:
	docker compose up -d --build --wait

# stop them (the data stays in its volume)
down:
	docker compose down

# what is running, and how each service says it is doing
ps:
	@docker compose ps --format "table {{.Service}}\t{{.Status}}"

# restart the API servers one at a time: the next one goes when the last one is ready again
roll:
	@for c in $$(docker compose ps -q api); do \
		docker restart $$c > /dev/null; \
		until [ "$$(docker inspect -f '{{.State.Health.Status}}' $$c)" = healthy ]; do sleep 0.2; done; \
		echo "$$(docker inspect -f '{{.Name}}' $$c | cut -c2-) is back, and ready"; \
	done

# one API server by hand, outside Docker, to work on it: it builds into bin/ and listens on 8081.
# nginx doesn't know it, so talk to it directly: curl localhost:8081/users/user1/posts
# the rate limits are settings: make run USER_BURST=3 USER_PER_MINUTE=12
# metrics, the profiler, /healthz and /readyz are on its private port, 9081
run:
	go build -o bin/api ./cmd/api
	./bin/api

# build the worker into bin/ and run it. Its private port is 9101
worker:
	go build -o bin/worker ./cmd/worker
	./bin/worker

# one package at a time: the tests share one database
test:
	go test -race -p 1 ./...

# fill the database with a graph of 100,000 users (it empties it first)
seed:
	go run ./cmd/seed --users 100000 --seed 1

# run a k6 script from load/, in Docker: make load S=feed
# it goes through nginx. Past nginx, straight to the API servers: make load S=feed BASE_URL=http://api:8081
# a like storm on one post: make load S=likes POST=2000002
# the mix at 1,500 requests a second, for a minute: make load S=mix RATE=1500 TIME=60s
load:
	docker compose run --rm -e BASE_URL -e VUS -e READERS -e AUTHORS -e FIRST -e POSTS -e RATE -e POST \
		-e TIME -e SPIKE -e STAR k6 run /load/$(S).js

# what Postgres does to build one user's feed: make explain U=42
Q ?= feed
explain:
	@docker compose exec -T postgres psql -U murmur -v user=$(U) < db/explain/$(Q).sql \
		| grep -E "Scan|Execution"

# one Redis command, run in the container: make redis C="dbsize"
redis:
	@docker compose exec redis redis-cli $(C)

# the queue: how many jobs are waiting, running and dead, and two of them: running ones first
jobs:
	@docker compose exec postgres psql -U murmur \
		-c "select count(*) filter (where status = 'waiting') as waiting, \
			count(*) filter (where status = 'running') as running, \
			count(*) filter (where status = 'dead') as dead, \
			max(now() - created_at) filter (where status <> 'dead')::interval(0) as oldest from jobs" \
		-c "select id, kind, status, payload from jobs where status <> 'dead' order by status, id limit 2"

# the dead list: jobs that used up their attempts, and the last thing that went wrong
dead:
	@docker compose exec postgres psql -U murmur \
		-c "select id, kind, attempts, left(last_error, 84) as last_error \
			from jobs where status = 'dead' order by id"

# send dead jobs back to the queue with a fresh count: all of them, or make retry ID=7
retry:
	@docker compose exec postgres psql -U murmur \
		-c "update jobs set status = 'waiting', attempts = 0, run_at = now() \
			where status = 'dead' $(if $(ID),and id = $(ID))"

# a job that can never work: its author is a name, and the handler wants a number
poison:
	@docker compose exec postgres psql -U murmur \
		-c "insert into jobs (kind, payload) \
			values ('fanout', jsonb_build_object('author', 'alice', 'post', 1))"

# a worker, played by hand: begin, pick a job with db/claim.sql, hold it for 8 seconds, roll back
claim:
	@docker compose exec -T postgres psql -U murmur -q \
		-c 'begin' -c '\timing on' -f - -c '\timing off' -c '\! sleep 8' -c 'rollback' < db/claim.sql

.PHONY: watch backlog cpu waits hold head likes followers

# the queue, every two seconds: jobs waiting, jobs running, and how long the oldest has waited
watch:
	@echo "select 'waiting ' || lpad((count(*) filter (where status = 'waiting'))::text, 6) \
		|| '   running ' || lpad((count(*) filter (where status = 'running'))::text, 3) \
		|| '   oldest ' || coalesce(extract(epoch from max(now() - created_at) \
		filter (where status <> 'dead'))::int, 0) || ' s' from jobs \\watch 2" \
		| docker compose exec -T postgres psql -U murmur -At

# work for the queue without anybody posting: the newest N posts of small accounts are fanned out
# once more, which changes no timeline. BIG=1 takes posts of big accounts: jobs with nothing to push
N ?= 10000
backlog:
	@docker compose exec -T postgres psql -U murmur -c \
		"insert into jobs (kind, payload) \
		select 'fanout', jsonb_build_object('author', p.user_id, 'post', p.id) \
		from posts p join users u on u.id = p.user_id \
		where u.followers_count $(if $(BIG),>=,<) 1000 order by p.id desc limit $(N)"

# how busy Postgres and Redis are, right now: 100% is one core
cpu:
	@docker compose stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}"

# what Postgres is doing: twenty looks at the statements that are running, and what each one waits for
waits:
	@for i in $$(seq 20); do docker compose exec -T postgres psql -U murmur -Atc \
		"select coalesce(wait_event, 'working') from pg_stat_activity \
		where state = 'active' and backend_type = 'client backend' and pid <> pg_backend_pid()"; \
	done | sort | uniq -c | sort -rn

# a transaction that reads once and then stays open, the way a backup does: make hold FOR=35
hold:
	@docker compose exec -T postgres psql -U murmur -q -c 'begin isolation level repeatable read' \
		-c 'select count(*) as users from users' -c '\! sleep $(FOR)' -c 'rollback'

# what one claim has to read before it finds its job: the pages it touched, and the time it took
head:
	@docker compose exec -T postgres psql -U murmur -c "explain (analyze, buffers, costs off, timing off) \
		select id from jobs where status = 'waiting' and run_at <= now() \
		order by run_at, id limit 1 for update skip locked" \
		| grep -E -A 3 "Scan|Execution" | grep -E "Buffers|Execution" | sed 's/^ *//'

# one post's likes: the rows in the likes table, the count on the post, and what still waits to be counted
likes:
	@docker compose exec -T postgres psql -U murmur -c \
		"select (select count(*) from likes where post_id = p.id) as likes, p.likes_count as counted, \
			(select coalesce(sum(delta), 0) from like_deltas where post_id = p.id) as waiting \
		from posts p where p.id = $(P)"

# the ids of everybody who follows one account, for the stream client:
# ./bin/listen -count $(make followers OF=115)
followers:
	@docker compose exec -T postgres psql -U murmur -Atc \
		"select follower_id from follows where followee_id = $(OF) order by 1"

.PHONY: ramp statements

# the mix at one rate after another, half a minute each: make ramp RATES="1000 2000 3000"
# between two steps it rests, so that a laptop can cool down
RATES ?= 1000 1500 2000 2500 3000 3500
REST ?= 30
ramp:
	@for r in $(RATES); do \
		docker compose run --rm -T -e BASE_URL -e RATE=$$r k6 run -q /load/mix.js 2> /dev/null; \
		sleep $(REST); \
	done

# what Postgres spent its time on, statement by statement, since the last time anybody asked:
# it prints the list, and then it starts counting from zero again
statements:
	@docker compose exec -T postgres psql -U murmur -q \
		-c "set client_min_messages = warning" \
		-c "create extension if not exists pg_stat_statements" \
		-c "select left(regexp_replace(query, '\s+', ' ', 'g'), 52) as statement, calls, plans, \
			round(total_plan_time) as plan_ms, round(total_exec_time) as exec_ms \
			from pg_stat_statements order by total_plan_time + total_exec_time desc limit 6" \
		-c "select pg_stat_statements_reset()" | grep -E "\||-\+-"
