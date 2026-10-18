# Stage one builds our programs, and then it's thrown away.
FROM golang:1.26.5-alpine AS build
WORKDIR /src

# The modules first: built again only when go.mod or go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/ ./cmd/api ./cmd/worker

# Stage two is what runs.
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/ /
ENTRYPOINT ["/api"]
