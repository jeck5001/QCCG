# ==========================================
# Stage 1: Build frontend
# ==========================================
FROM node:22-alpine AS frontend-builder

WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ==========================================
# Stage 2: Build Go binary (headless bridge mode)
# ==========================================
FROM golang:1.25-alpine AS go-builder

ARG VERSION=0.0.0-dev

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Cache Go module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy frontend dist from previous stage
COPY --from=frontend-builder /app/frontend/dist frontend/dist/

# Build headless bridge binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags headless -trimpath \
    -ldflags="-w -s -X qccg/internal/updater.Version=${VERSION}" \
    -o qccg-bridge .

# ==========================================
# Stage 3: Runtime image
# ==========================================
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS requests and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=go-builder /app/qccg-bridge .

# Create data directory
RUN mkdir -p /data/.qccg/logs /data/.qccg/accounts

# Environment variables (configured at runtime)
ENV QODER_PAT="" \
    QCCG_REGION="global" \
    QCCG_PORT="8963" \
    QCCG_BRIDGE_TOKEN="qccg" \
    QCCG_LISTEN="0.0.0.0" \
    QCCG_DATA_DIR="/data" \
    QCCG_LOG_LEVEL="info"

# Expose bridge port
EXPOSE 8963

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8963/health || exit 1

# Data volume
VOLUME /data

ENTRYPOINT ["./qccg-bridge"]
