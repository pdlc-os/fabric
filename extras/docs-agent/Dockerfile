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

# Stage 1: Clone repo and build the handler
FROM golang:1.25 AS builder

WORKDIR /build

# Clone the Fabric repo at build time for latest content
ARG FABRIC_REPO=https://github.com/pdlc-os/fabric.git
ARG FABRIC_REF=main
RUN git clone --depth 1 --branch ${FABRIC_REF} ${FABRIC_REPO} /fabric-source

# Build the docs-agent handler
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /docs-agent .

# Stage 2: Runtime
FROM node:20-slim

# Install git (needed for /refresh endpoint) and Gemini CLI
RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @google/gemini-cli@latest \
    && npm cache clean --force

# Copy handler binary and source context
COPY --from=builder /docs-agent /usr/local/bin/docs-agent
COPY --from=builder /fabric-source /workspace/fabric

EXPOSE 8080
CMD ["docs-agent"]
