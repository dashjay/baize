#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

pseudo_version="${STABLE_DOCKER_TAG_OVERRIDE:-}"
if [ -z "$pseudo_version" ]; then
  pseudo_version=${BRAIN_GIT_VERSION/+/-}
  if [ -z "$pseudo_version" ]; then
    # if "$pseudo_version" is empty use original version format
    git_commit_date="$(git show -s --format=%cd --date=format:%Y%m%d%H%M%S HEAD)"
    git_commit_hash="$(git log --format=%H -n 1 | head -c 12)"
    pseudo_version="v0.0.0-${git_commit_date}-${git_commit_hash}"
    if git_status=$(git status --porcelain 2>/dev/null) && [[ -n ${git_status} ]]; then
      pseudo_version+="-dirty"
    fi
  fi
fi

cat <<EOF
STABLE_DOCKER_TAG ${pseudo_version}
EOF
