# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and database migrations
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server/main.go

# Final stage
FROM alpine:3.19

# Add CA certificates and time zones
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy compiled binary and migrations from the builder
COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations

# Expose HTTP port
EXPOSE 8080

# Command to run the application
CMD ["./server"]
