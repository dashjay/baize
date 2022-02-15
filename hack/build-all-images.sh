#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
rootDir="$currentDir"/..
readonly set_workspace_env=$("$rootDir"/print-workspace-status.sh | sed -E "s/^([^ ]+) (.*)\$$/export \\1=\\2/g")
$set_workspace_env

cd "$rootDir"

docker build -t dashjay/ubuntu:executor-base . -f base.Dockerfile
docker build -t dashjay/baize-executor:"${STABLE_DOCKER_TAG}" . -f executor.Dockerfile
docker build -t dashjay/baize-server:"${STABLE_DOCKER_TAG}" . -f server.Dockerfile