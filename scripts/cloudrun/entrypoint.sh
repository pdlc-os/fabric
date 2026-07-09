#!/bin/sh
set -e
# Copy secret-mounted settings into ~/.fabric/ so the runtime discovery finds them.
# Cloud Run secret volumes use symlink-based atomic updates, so cp may fail.
# Use cat to read through the symlink safely.
mkdir -p "$HOME/.fabric/storage" "$HOME/.fabric/templates"
if [ -f /run/secrets/settings.yaml ]; then
  cat /run/secrets/settings.yaml > "$HOME/.fabric/settings.yaml"
fi
exec fabric server start \
  --foreground --production \
  --enable-hub --enable-runtime-broker --enable-web --web-port 8080 \
  --auto-provide --global
