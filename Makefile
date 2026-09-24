# Local development. The API reads DATABASE_URL and REDIS_URL from the environment.
DATABASE_URL ?= postgres://murmur:murmur@localhost:5432/murmur?sslmode=disable&connect_timeout=5
REDIS_URL ?= redis://localhost:6379/0
export DATABASE_URL REDIS_URL

.PHONY: up down run test seed load explain redis worker jobs claim

# start Postgres and Redis
up:
	docker compose up -d --wait

# stop them (the data stays in its volume)
down:
	docker compose down

# build the API into bin/ and run it
run:
	go build -o bin/api ./cmd/api
	./bin/api

# build the worker into bin/ and run it
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
load:
	docker compose run --rm -e VUS -e READERS -e AUTHORS -e FIRST k6 run /load/$(S).js

# what Postgres does to build one user's feed: make explain U=42
Q ?= feed
explain:
	@docker compose exec -T postgres psql -U murmur -v user=$(U) < db/explain/$(Q).sql \
		| grep -E "Scan|Execution"

# one Redis command, run in the container: make redis C="dbsize"
redis:
	@docker compose exec redis redis-cli $(C)

# the queue: how many jobs are in it, how long the oldest has waited, and the first two
jobs:
	@docker compose exec postgres psql -U murmur \
		-c "select count(*) as jobs, max(now() - created_at)::interval(0) as oldest from jobs" \
		-c "select id, kind, status, payload from jobs order by id limit 2"

# a worker, played by hand: begin, pick a job with db/claim.sql, hold it for 8 seconds, roll back
claim:
	@docker compose exec -T postgres psql -U murmur -q \
		-c 'begin' -c '\timing on' -f - -c '\timing off' -c '\! sleep 8' -c 'rollback' < db/claim.sql
