#!/usr/bin/env bash
# update go dependencies, vendor them and cleanup bazel build files
set -o errexit
set -o nounset
set -o pipefail
set -x

# clear dependencies
true >go.mod.bzl
true >go.sum

# verify and vendor all dependencies
go mod verify
go mod tidy -v
go mod vendor

# remove all files that were fetched with previous commands in order to avoid conflicts with proto rules
find vendor -iname BUILD.bazel -delete

# update repositories
bazel run //:gazelle -- update-repos -from_file=go.mod -to_macro=go.mod.bzl%go_repositories

# beautify bazel files
find . -type f -iname BUILD.bazel -o -name WORKSPACE | while IFS='' read -r buildfile; do
  buildifier "${buildfile}"
done

# create and update build files
bazel run //:gazelle
