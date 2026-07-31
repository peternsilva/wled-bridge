#!/bin/sh
DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
export PATH="$DIR:$PATH"
exec "$DIR/wb" "$@"
