# ----------------------------------------------------------
# Builder stage
# ----------------------------------------------------------
FROM golang:1.25.0 AS builder

WORKDIR /app

# 1) deps layer
COPY go.mod go.sum ./
# Use BuildKit cache mounts to persist module cache and build cache
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# 2) source layer
# Copy only what is needed to build
COPY ./api ./api
COPY ./database ./database
COPY ./docs ./docs
COPY ./utils ./utils
COPY ./main.go ./

# Build static binary
# -trimpath removes local paths, -ldflags reduces size
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /app/bin/app

# ----------------------------------------------------------
# Runner
# ----------------------------------------------------------
FROM scratch AS runner
WORKDIR /
COPY --from=builder /app/bin/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
