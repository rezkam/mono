# Build Stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 for static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o mono-server cmd/server/main.go

# Final Stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mono-server .

# Copy migration files (if needed for runtime migration)
COPY --from=builder /app/internal/infrastructure/persistence/postgres/migrations ./migrations

# Use non-root user
USER appuser

# Expose ports
EXPOSE 8080 8081

# Set entrypoint
ENTRYPOINT ["./mono-server"]
