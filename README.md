# wled-bridge

Translate qwerty, MIDI, and Xbox 360 controller input into WLED HTTP preset calls.

Requires a bundled `midicat` 0.9.x binary (for `midicatdrv`) next to the app.

## Install on Raspberry Pi OS Lite (Trixie, arm64)

Copy the contents of `deploy/` to the Pi (that folder alone is enough — no Go, no full git clone required), then:

```sh
cd /path/to/deploy   # directory with wb, midicat, run.sh, install.sh, wled-bridge.service
sudo ./install.sh
```

That installs into `/opt/wled-bridge`, enables `wled-bridge.service` at boot with `Restart=always`, and starts it immediately.

## Develop with `go run`

`midicatdrv` requires `midicat` **0.9.x** on your `PATH` before the process starts. The bundled binary is in `deploy/`:

```sh
export PATH="$PWD/deploy:$PATH"
go run .
# or: ./deploy/run.sh -verbose   # after: ./build.sh
```

If an older `midicat` (e.g. 0.8.2 in `~/go/bin`) is earlier on `PATH`, replace it or put `deploy/` first.

## Rebuild binaries (developer Pi)

```sh
./build.sh
# prints e.g. built deploy/wb version main-a1b2c3d
cat deploy/VERSION
./deploy/wb -version
# refresh midicat from a local linux-arm64 0.9.5 build if needed:
# cp /path/to/midicat deploy/midicat && chmod +x deploy/midicat
```

Version strings are `BRANCH-commit` (7-char SHA), with `-dirty` when the tree has uncommitted changes.
