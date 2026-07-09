# syntax=docker/dockerfile:1
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Fabric Base Image - Adds fabrictool and fabric user on top of core-base
ARG BASE_IMAGE
FROM ${BASE_IMAGE} AS base

# Builder stage for fabrictool compilation
FROM base AS builder
USER root
WORKDIR /src
ENV PATH="/usr/local/go/bin:${PATH}"
ENV CGO_ENABLED=1

# Build arguments for version info
ARG GIT_COMMIT
ARG BUILD_TIME

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build fabric and fabrictool
COPY cmd/ ./cmd/
COPY harnesses/ ./harnesses/
COPY pkg/ ./pkg/
COPY resources/ ./resources/
COPY web/*.go ./web/

# Build with version info injected via ldflags
# VCS stamping is disabled since .git is not in the build context;
# commit info is passed explicitly via the GIT_COMMIT build arg
RUN BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    FABRIC_PKG="github.com/pdlc-os/fabric/pkg/version" && \
    FABRICTOOL_PKG="github.com/pdlc-os/fabric/cmd/fabrictool/commands" && \
    FABRIC_LDFLAGS="-s -w -X ${FABRIC_PKG}.BuildTime=${BUILD_TIME}" && \
    FABRICTOOL_LDFLAGS="-s -w -X ${FABRICTOOL_PKG}.BuildTime=${BUILD_TIME}" && \
    if [ -n "${GIT_COMMIT}" ]; then \
        FABRIC_LDFLAGS="${FABRIC_LDFLAGS} -X ${FABRIC_PKG}.Commit=${GIT_COMMIT}"; \
        FABRICTOOL_LDFLAGS="${FABRICTOOL_LDFLAGS} -X ${FABRICTOOL_PKG}.Commit=${GIT_COMMIT}"; \
    fi && \
    go build -buildvcs=false -tags no_embed_web -ldflags="${FABRIC_LDFLAGS}" -o /usr/local/bin/fabric ./cmd/fabric/ && \
    go build -buildvcs=false -tags no_embed_web -ldflags="${FABRICTOOL_LDFLAGS}" -o /usr/local/bin/fabrictool ./cmd/fabrictool/

# Final stage
FROM base
USER root

# Copy fabric and fabrictool from builder
COPY --from=builder /usr/local/bin/fabric /usr/local/bin/fabric
COPY --from=builder /usr/local/bin/fabrictool /usr/local/bin/fabrictool

# Set up gh CLI wrapper for GitHub App token injection.
# Move the real gh binary aside and install a shell wrapper that delegates
# through fabrictool gh-wrapper, which reads the latest token from the
# refreshable token file before exec-ing the real binary.
RUN if [ -f /usr/bin/gh ]; then \
      mv /usr/bin/gh /usr/bin/gh.real && \
      printf '#!/bin/sh\nexec fabrictool gh-wrapper "$@"\n' > /usr/bin/gh && \
      chmod +x /usr/bin/gh; \
    fi

# Create dev-override bin directory at highest PATH priority.
# When FABRIC_DEV_BINARIES is set, this directory is bind-mounted with
# locally-built binaries, allowing rapid iteration without image rebuilds.
RUN mkdir -p /opt/fabric/bin
ENV PATH="/opt/fabric/bin:${PATH}"

# Create fabric user with passwordless sudo
RUN useradd -m -s /bin/zsh -u 1000 fabric && \
    echo "fabric ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/fabric && \
    chmod 0440 /etc/sudoers.d/fabric

# Precreate shared Fabric home state so mounted file secrets and runtime
# config writes do not force root-owned parent directories at startup.
RUN mkdir -p /home/fabric/.fabric && \
    chown -R fabric:fabric /home/fabric

# Set ownership for npm global directory
# TODO: Consider making npm-global world read-write (chmod) instead of chown
# to fabric. This would decouple it from the fabric UID and avoid any ownership
# concerns when the UID is remapped at container start.
RUN chown -R fabric:fabric /usr/local/share/npm-global

# Note: Container starts as root to allow fabrictool init to adjust
# the fabric user's UID/GID to match the host user for proper file
# permissions on mounted volumes. The child process drops privileges.

# Set fabrictool as the container entrypoint (PID 1)
# The -- separator ensures everything after it is treated as the child command
ENTRYPOINT ["fabrictool", "init", "--"]

# Default command (can be overridden at runtime)
CMD ["bash"]
