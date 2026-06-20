# =============================================================================
# Multi-Stage Dockerfile for E-Letter Golang/Gin Backend
# =============================================================================

# ─────────────────────────────────────────────────────────────────────────────
# Stage 1: Build
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# Install build tools required by CGO-less builds
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go module files first to leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build a statically linked binary (CGO_ENABLED=0 for Alpine compatibility)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bin/eletter-api ./cmd/api

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2: Minimal runtime image
# ─────────────────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS runtime

# wget is needed for the docker-compose healthcheck
RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser  -S appuser -u 1001 -G appgroup

WORKDIR /app

# Create uploads and signatures directories with correct ownership for volume mount
RUN mkdir -p /app/public/uploads/signatures && chown -R appuser:appgroup /app/public

# Copy compiled binary from builder
COPY --from=builder --chown=appuser:appgroup /bin/eletter-api ./eletter-api

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=5 \
    CMD wget --quiet --tries=1 --spider -O /dev/null http://localhost:8080/health || exit 1

ENTRYPOINT ["./eletter-api"]
