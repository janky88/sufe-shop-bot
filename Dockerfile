# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o shopbot ./cmd/server

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 shopbot && \
    adduser -u 1000 -G shopbot -s /bin/sh -D shopbot

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/shopbot /app/shopbot

# Copy static files and templates
COPY --from=builder /build/templates /app/templates
COPY --from=builder /build/static /app/static

# Create directories for logs and data
RUN mkdir -p /app/logs /app/data && \
    chown -R shopbot:shopbot /app

# Switch to non-root user
USER shopbot

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Set environment variables
ENV CONFIG_PATH=/app/config.yaml

# Run the application
CMD ["/app/shopbot"]