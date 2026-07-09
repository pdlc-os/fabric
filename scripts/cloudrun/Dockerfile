# Fabric Hub — Cloud Run container image
# Multi-stage build: web frontend → Go binary → slim runtime

# ---------------------------------------------------------------------------
# Stage 1: Build web frontend
# ---------------------------------------------------------------------------
FROM node:20-slim AS web-builder

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

# ---------------------------------------------------------------------------
# Stage 2: Build Go binary (with embedded web assets)
# ---------------------------------------------------------------------------
FROM golang:1.25 AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/web/dist/client ./web/dist/client

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -buildvcs=false \
      -ldflags "-X github.com/pdlc-os/fabric/pkg/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      -o /fabric ./cmd/fabric

# ---------------------------------------------------------------------------
# Stage 3: Runtime
# ---------------------------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      git \
      openssh-client \
      curl \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN useradd -m -d /home/fabric -s /bin/bash -u 1000 fabric \
    && mkdir -p /home/fabric/.kube /run/secrets \
    && chown -R fabric:fabric /home/fabric /run/secrets

COPY --from=go-builder /fabric /usr/local/bin/fabric
COPY scripts/cloudrun/entrypoint.sh /usr/local/bin/entrypoint.sh

ENV HOME=/home/fabric
ENV KUBECONFIG=/home/fabric/.kube/config

USER fabric
WORKDIR /home/fabric

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
