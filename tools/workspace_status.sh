#!/usr/bin/env bash
set -euo pipefail

# Git tag-based version (strip leading 'v')
STABLE_VERSION=$(git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
echo "STABLE_VERSION ${STABLE_VERSION}"

# Git commit SHA
echo "STABLE_GIT_COMMIT $(git rev-parse HEAD 2>/dev/null)"

# Build date in ISO 8601
echo "BUILD_DATE $(date -u +%Y-%m-%dT%H:%M:%SZ)"
