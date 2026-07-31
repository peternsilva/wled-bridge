# wled-bridge

Translate qwerty, MIDI, and Xbox 360 controller input into WLED HTTP preset calls.

Requires a bundled `midicat` 0.9.x binary (for `midicatdrv`) next to the app.

## Install on Raspberry Pi OS Lite (Trixie, arm64)

From a clone of this repo:

```sh
sudo ./install.sh
```

That copies `deploy/` into `/opt/wled-bridge`, enables `wled-bridge.service` at boot with `Restart=always`, and starts it immediately.

## Rebuild binaries (developer Pi)

```sh
CGO_ENABLED=0 go build -o deploy/wb .
# refresh midicat from a local linux-arm64 0.9.5 build if needed:
# cp /path/to/midicat deploy/midicat && chmod +x deploy/midicat
```
