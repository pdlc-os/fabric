#!/usr/bin/env bash
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

# Render the Homebrew formula template with a release tag and the sha256
# checksums of that release's tarballs.
#
# Usage: render.sh <tag> <dir-with-release-tarballs> > fabric.rb
#
# <dir> must contain the four release assets:
#   fabric-darwin-arm64.tar.gz  fabric-darwin-amd64.tar.gz
#   fabric-linux-arm64.tar.gz   fabric-linux-amd64.tar.gz
set -euo pipefail

if [ $# -ne 2 ]; then
    echo "usage: $0 <tag> <tarball-dir>" >&2
    exit 1
fi

TAG="$1"
DIR="$2"
VERSION="${TAG#v}"
TEMPLATE="$(dirname "$0")/fabric.rb.tmpl"

sha() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$DIR/$1" | awk '{print $1}'
    else
        shasum -a 256 "$DIR/$1" | awk '{print $1}'
    fi
}

sed \
    -e "s|{{VERSION}}|${VERSION}|g" \
    -e "s|{{TAG}}|${TAG}|g" \
    -e "s|{{SHA_DARWIN_ARM64}}|$(sha fabric-darwin-arm64.tar.gz)|" \
    -e "s|{{SHA_DARWIN_AMD64}}|$(sha fabric-darwin-amd64.tar.gz)|" \
    -e "s|{{SHA_LINUX_ARM64}}|$(sha fabric-linux-arm64.tar.gz)|" \
    -e "s|{{SHA_LINUX_AMD64}}|$(sha fabric-linux-amd64.tar.gz)|" \
    "$TEMPLATE"
