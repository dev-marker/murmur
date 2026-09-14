# Local development. The API reads DATABASE_URL from the environment.
DATABASE_URL ?= postgres://murmur:murmur@localhost:5432/murmur?sslmode=disable&connect_timeout=5
export DATABASE_URL

.PHONY: up down run test seed load explain

# start Postgres and wait until it is healthy
up:
	docker compose up -d --wait

# stop Postgres (the data stays in its volume)
down:
	docker compose down

# build the API into bin/ and run it
run:
	go build -o bin/api ./cmd/api
	./bin/api

test:
	go test -race ./...

# fill the database with a graph of 100,000 users (it empties it first)
seed:
	go run ./cmd/seed --users 100000 --seed 1

# run a k6 script from load/, in Docker: make load S=feed
load:
	docker compose run --rm -e VUS k6 run /load/$(S).js

# what Postgres does to build one user's feed: make explain U=42
explain:
	@docker compose exec -T postgres psql -U murmur -v user=$(U) < db/explain/feed.sql \
		| grep -E "Scan|Execution"
