# ----------------------------------------------------------
# Builder stage
# ----------------------------------------------------------
FROM golang:1.22 AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application files
COPY ./config ./config
COPY ./controllers ./controllers
COPY ./database ./database
COPY ./docs ./docs
COPY ./middlewares ./middlewares
COPY ./models ./models
COPY ./routes ./routes
COPY ./utils ./utils
COPY ./main.go ./

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app 

# ----------------------------------------------------------
# Runner stage
# ----------------------------------------------------------
FROM alpine:latest

WORKDIR /root/

# Copy the compiled binary from builder
COPY --from=builder /app/app .

# Expose port 8080
EXPOSE 8080

# Run the binary
CMD ["./app"]
