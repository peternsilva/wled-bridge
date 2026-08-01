#!/bin/sh
# Build wled-bridge into deploy/wb with a BRANCH-commit version.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
cd "$ROOT"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	echo "build.sh: not a git checkout" >&2
	exit 1
fi

BRANCH=$(git rev-parse --abbrev-ref HEAD | tr '/' '-' | tr -c 'A-Za-z0-9._-' '-')
# Collapse runs of dashes from sanitization and strip leading/trailing dashes.
BRANCH=$(printf '%s' "$BRANCH" | sed -e 's/-\+/-/g' -e 's/^-//' -e 's/-$//')
if [ -z "$BRANCH" ]; then
	BRANCH=HEAD
fi

COMMIT=$(git rev-parse --short=7 HEAD)
VERSION="${BRANCH}-${COMMIT}"
if [ -n "$(git status --porcelain)" ]; then
	VERSION="${VERSION}-dirty"
fi

mkdir -p deploy
CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o deploy/wb .
printf '%s\n' "$VERSION" > deploy/VERSION

echo "built deploy/wb version ${VERSION}"
