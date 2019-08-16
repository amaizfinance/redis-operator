#!/usr/bin/env bash
# Build time variable to embed. Should be passed as --workspace_status_command for bazel
# Refs:
# - https://github.com/bazelbuild/rules_go/blob/master/go/core.rst#defines-and-stamping
# - https://docs.drone.io/reference/environ/
set -o errexit
set -o nounset
set -o pipefail

cat <<EOF
ImageTag ${DRONE_TAG:-latest}
Version ${DRONE_TAG:-latest}
GitCommit ${DRONE_COMMIT_SHA:-$(git rev-parse HEAD)}
BuildDate $(date -u +'%Y-%m-%dT%H:%M:%SZ')
EOF
