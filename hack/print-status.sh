#!/usr/bin/env bash
# Build time variable to embed. Should be passed as --workspace_status_command for bazel
# Refs:
# - https://github.com/bazelbuild/rules_go/blob/master/go/core.rst#defines-and-stamping
# - https://docs.drone.io/reference/environ/
set -o errexit
set -o nounset
set -o pipefail

cat <<EOF
Version ${DRONE_TAG:-latest}
ImageTag ${DRONE_TAG:-latest}
EOF
