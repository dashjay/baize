#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
rootDir="$currentDir"/../..
set_workspace_env=$("$rootDir"/hack/print-workspace-status.sh | sed -E "s/^([^ ]+) (.*)\$$/export \\1=\\2/g")
readonly set_workspace_env
$set_workspace_env

docker-compose -f "$currentDir"/docker-compose.yaml down "$@"