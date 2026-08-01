# wled-bridge

Turns keyboard, Xbox 360 controller, and Launchpad MIDI into WLED light presets.

## Install (Raspberry Pi)

1. Copy everything inside the `deploy/` folder onto the Pi.
2. On the Pi, open a terminal in that folder and run:

```sh
sudo ./install.sh
```

That installs the app, starts it, and sets it to start again whenever the Pi boots.

To check that it’s running:

```sh
systemctl status wled-bridge
```

To see which build is installed:

```sh
cat /opt/wled-bridge/VERSION
```

## Developer notes

**What it needs:** the `midicat` binary next to the app (`deploy/midicat`, version 0.9.x). The install script puts both under `/opt/wled-bridge`.

**Run from source:**

```sh
export PATH="$PWD/deploy:$PATH"
go run .
# or after building: ./deploy/run.sh -verbose
```

If an older `midicat` (e.g. 0.8.2 in `~/go/bin`) is earlier on `PATH`, put `deploy/` first or replace that binary.

**Rebuild the app binary:**

```sh
./build.sh
cat deploy/VERSION
./deploy/wb -version
```

Versions look like `BRANCH-commit` (7-char SHA), with `-dirty` if there are uncommitted changes.

**Refresh midicat** (linux-arm64 0.9.5), if needed:

```sh
cp /path/to/midicat deploy/midicat && chmod +x deploy/midicat
```

Target: Raspberry Pi OS Lite (Trixie, arm64). Copying only `deploy/` is enough — no Go toolchain or full git clone required on the install Pi.
