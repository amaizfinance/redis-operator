#!/usr/bin/env bash
# build and push operator image
set -o errexit
set -o nounset
set -o pipefail

DOCKER_CONFIG=/tmp
export DOCKER_CONFIG
echo "${DOCKER_AUTH}" >"${DOCKER_CONFIG}/config.json"

bazel run --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 //cmd/manager:push_manager_image
