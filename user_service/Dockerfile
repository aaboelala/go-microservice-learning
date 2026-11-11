# Multi-stage build
#
# WHY multi-stage?
# Stage 1 (builder): has Go toolchain (~800MB), compiles the binary
# Stage 2 (runtime): has ONLY the binary (~15MB)
#
# Without multi-stage, the final image would include the Go compiler,
# source code, and all build dependencies — wasting ~800MB of disk and
# increasing the attack surface.

# --- Stage 1: Build ---
FROM golang:1.24-alpine AS builder

# Install git (needed for go mod download with private repos)
RUN apk add --no-cache git

WORKDIR /app

# Copy go.mod and go.sum first — Docker caches this layer.
# If dependencies haven't changed, Docker skips `go mod download`.
# This saves minutes on rebuilds when only source code changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0: static binary, no C library dependency
# -o /app/server: output binary path
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

# --- Stage 2: Runtime ---
FROM alpine:3.21

# Install CA certificates (needed for HTTPS connections)
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy ONLY the compiled binary from the builder stage
COPY --from=builder /app/server .

# Run as non-root user for security
RUN adduser -D -u 1001 appuser
USER appuser

EXPOSE 8080

CMD ["./server"]
