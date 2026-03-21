# GoGate — Multi-stage Dockerfile
# Produces a minimal scratch-based image with no shell, no package manager.
#
# Build:
#   docker build -f deployments/docker/Dockerfile -t gogate:latest .
#
# Run:
#   docker run -p 8080:8080 -v $(pwd)/config.yaml:/etc/gogate/config.yaml gogate:latest

# ── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.date=${BUILD_DATE}" \
    -o /bin/gateway ./cmd/gateway

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM scratch

# Import CA certificates and timezone data from builder.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Run as non-root (UID 65534 = nobody).
USER 65534:65534

# Copy binary.
COPY --from=builder /bin/gateway /bin/gateway

# Default config path (mount your config here).
ENV GOGATE_CONFIG=/etc/gogate/config.yaml

EXPOSE 8080

ENTRYPOINT ["/bin/gateway"]
