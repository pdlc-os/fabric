---
title: Onboarding Wizard
description: A guided, browser-based walkthrough that sets up your workstation the first time you run fabric server start — identity, system checks, runtime, images, harnesses, and your first project.
---

**What you will learn**: How to set up Fabric in [Workstation mode](/fabric/choosing-a-mode/) using the browser-based onboarding wizard — from a fresh install to your first project — without touching a config file.

The onboarding wizard is the fastest way to get the hosted experience running locally. When you start the Workstation combo server for the first time, Fabric opens a guided setup in your browser that walks through machine configuration, environment checks, runtime selection, harness images, and creating your first project.

:::note[Which mode is this?]
The wizard sets up **Workstation mode** — a single-tenant Fabric server (Hub + Runtime Broker + Web dashboard) running on your own machine over loopback. If you only want to run agents from the CLI with no server, see [Local mode](/fabric/choosing-a-mode/) and the [Installation guide](/fabric/getting-started/install/) instead.
:::

## Prerequisites

Before you start, you need a working Fabric install and a container runtime. See the [Installation guide](/fabric/getting-started/install/) for details. In short:

- Fabric installed with its web assets. Build from a clone with `make all` for a ready-to-run install; a bare `go install` does **not** embed the web UI, so the wizard would load blank. (Homebrew installation is temporarily unavailable while the formula catches up with the rename to Fabric.)
- A container runtime — Docker, Podman, or Apple Container.
- Git 2.47 or later (the wizard flags older versions).

You do **not** need to run `fabric init --machine` first — the wizard handles machine initialization for you.

## Launching the wizard

Start the Workstation server:

```bash
fabric server start
```

On a machine that has not been set up yet, Fabric prints the web URL and **automatically opens your browser** to the wizard:

```
http://127.0.0.1:8080/onboarding
```

If the browser does not open (for example over SSH, in a headless environment, or when `FABRIC_NO_BROWSER` is set), open that URL manually. The port is `8080` by default.

:::tip[You can leave and come back]
The wizard saves your progress. If you close the tab or restart setup, reopening `/onboarding` resumes past the steps you have already completed.
:::

## The steps

The wizard runs through six steps and finishes with a confirmation screen. Each step can be revisited with **Back**, and several can be skipped and completed later from the dashboard.

### 1. Welcome & identity

Enter a **display name** and **email**. This identity is attached to the agents and activity you create on this workstation. Provide at least one of the two to continue.

### 2. System check

The wizard runs diagnostics against your environment and shows each result as **pass**, **warn**, or **fail**. Use **Re-check** after fixing anything. A common warning is an out-of-date Git — Fabric needs **Git 2.47+** for agent worktrees; upgrade (for example `brew install git`) and re-check. You can only advance once the checks report ready.

### 3. Container runtime

Fabric detects the runtimes available on your machine and preselects the best one. Pick from **Docker**, **Podman**, or **Container (Apple Virtualization)**; runtimes that were not detected are shown but disabled.

:::caution[Apple Container needs DNS setup]
If you select **Container** (Apple Virtualization), the wizard attempts to configure container DNS, which requires `sudo`. If it cannot do so automatically, it shows the exact command to run manually, for example:

```bash
sudo container system dns create <hostname> --localhost <ip>
```
:::

### 4. Image registry

Enter the container image registry that hosts the Fabric harness images (for example `us-central1-docker.pkg.dev/my-project/fabric`). Images such as `<registry>/fabric-claude:latest` are pulled from here in the next step. If you are not ready, choose **Skip for now** and set it later.

### 5. AI harnesses

Select the [harnesses](/fabric/supported-harnesses/) you want available (Claude Code, Gemini CLI, Codex, OpenCode, and others). For each selected harness the wizard checks whether its container image is present:

- **ready** — the image is available locally.
- **available** — the image exists in the registry and can be pulled.
- **not found / error** — the image could not be located.

Use **Pull selected** to fetch any images that are not local yet (progress streams live), then **Re-check** to confirm. You can add or reconfigure harnesses later from Hub settings, or **Skip for now**.

### 6. First workspace

Create your first **project**. The wizard offers three ways to start:

- **Hub-managed project** — a workspace the Hub creates and manages for you; no git repository required.
- **Link a git repo** — connect an existing git repository for source-controlled workspaces.
- **Add local directory** — link a local directory that stays where it is and is operated on in place. The wizard validates the path and warns if it is already a git repo or already linked.

This step is optional — choose **Skip for now** to create projects later from the dashboard.

### You're all set

The final screen confirms your workstation is configured. Click **Go to Dashboard** to open the [Web Dashboard](/fabric/workstation/dashboard/) at `http://127.0.0.1:8080` and start running agents.

## After onboarding

- Manage the running server with `fabric server status`, `fabric server restart`, and `fabric server stop`. See [Workstation Server Mode](/fabric/workstation/workstation-server/) for the combo server, network bridges, and lifecycle details.
- Learn your way around the [Web Dashboard](/fabric/workstation/dashboard/).
- Understand the pieces you just configured in [Core Concepts](/fabric/concepts/).

## See also

- [Choosing a Mode](/fabric/choosing-a-mode/) — where Workstation mode fits among Local and the hosted tiers.
- [Installation](/fabric/getting-started/install/) — prerequisites and install methods.
- [Workstation Server Mode](/fabric/workstation/workstation-server/) — the server the wizard starts.
