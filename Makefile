# Local development. The API reads DATABASE_URL from the environment.
DATABASE_URL ?= postgres://murmur:murmur@localhost:5432/murmur?sslmode=disable&connect_timeout=5
export DATABASE_URL

.PHONY: up down run test

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
