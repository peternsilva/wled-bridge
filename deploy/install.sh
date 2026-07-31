#!/bin/sh
# Install wled-bridge on Raspberry Pi OS Lite (Trixie, arm64).
# This directory is self-contained: copy only these files to a new Pi, then:
#   sudo ./install.sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
	echo "run as root: sudo ./install.sh" >&2
	exit 1
fi

DEPLOY=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PREFIX=/opt/wled-bridge
UNIT_SRC="$DEPLOY/wled-bridge.service"
UNIT_DST=/etc/systemd/system/wled-bridge.service

for f in wb midicat run.sh wled-bridge.service; do
	if [ ! -e "$DEPLOY/$f" ]; then
		echo "missing $DEPLOY/$f" >&2
		exit 1
	fi
done

install -d "$PREFIX"
install -m 755 "$DEPLOY/wb" "$PREFIX/wb"
install -m 755 "$DEPLOY/midicat" "$PREFIX/midicat"
install -m 755 "$DEPLOY/run.sh" "$PREFIX/run.sh"
install -m 644 "$UNIT_SRC" "$UNIT_DST"

systemctl daemon-reload
systemctl enable --now wled-bridge.service

if ! systemctl is-active --quiet wled-bridge.service; then
	echo "wled-bridge.service failed to start:" >&2
	systemctl status wled-bridge.service --no-pager >&2 || true
	exit 1
fi

echo "installed to $PREFIX"
systemctl --no-pager --full status wled-bridge.service || true
