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

# ADK Fabric Agent — Container Image
#
# Builds on fabric-base which provides fabrictool (PID 1 init), the fabric user,
# tmux, git, and Python 3. This layer adds the google-adk package and the
# agent source code.
#
# Build:
#   docker build -t fabric-adk-agent .
#
# The ENTRYPOINT is inherited from fabric-base:
#   fabrictool init -- <CMD>
#
# At runtime, fabric wraps CMD in a tmux session so that `fabric message` can
# deliver user input via send-keys into ADK's interactive input() loop.

ARG BASE_IMAGE
FROM ${BASE_IMAGE}

USER root

# Create workspace directory
RUN mkdir -p /workspace && chown fabric:fabric /workspace
WORKDIR /workspace

# Install google-adk from source — the EnvironmentToolset and
# LocalEnvironment used by this agent are not yet in the released
# PyPI package.  Install from git HEAD until a release includes them.
COPY requirements.txt /tmp/requirements.txt
RUN python3 -m venv /opt/adk-venv && \
    /opt/adk-venv/bin/pip install --no-cache-dir -r /tmp/requirements.txt

# Put the venv on PATH so `adk` CLI is available to all users.
ENV PATH="/opt/adk-venv/bin:${PATH}"

# Copy the agent source code and ensure Python can find the package.
COPY --chown=fabric:fabric . /opt/adk_fabric_agent/adk_fabric_agent/
ENV PYTHONPATH="/opt/adk_fabric_agent:${PYTHONPATH}"

# Default command: run the ADK agent via the custom runner entrypoint.
# The custom runner supports --input for task delivery (which the released
# `adk run` CLI does not yet support). fabric's runtime layer wraps this in
# `tmux new-session -s fabric ...` and appends `--input <task>` when task_flag
# is configured.
CMD ["python", "-m", "adk_fabric_agent"]
