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

# Fabric Hub image - extends fabric-base to run the centralized hub/broker
# server. The fabric binary baked into fabric-base already contains the
# server subcommand (`fabric server start`); this layer just sets the
# default entrypoint and prepares hub-specific filesystem state.

ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Hub state lives under /home/fabric/.fabric (sqlite, templates, etc.).
# fabric-base already created this directory and chowned it; ensure it
# survives any image flattening here.
RUN mkdir -p /home/fabric/.fabric && \
  chown -R fabric:fabric /home/fabric/.fabric

# The fabric-base ENTRYPOINT runs fabrictool init -- <cmd>. Override the
# default CMD so the container, when started without arguments, runs the
# hub server in the foreground.
CMD ["fabric", "server", "start", "--foreground", "--host", "0.0.0.0", "--enable-hub"]
