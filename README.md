# tlr-time-sync

Keeps an SDR-Trunk machine's system clock synchronised with a TLR server.
Accurate timestamps mean the TLR duplicate-call detection window lines up precisely
with the times SDR-Trunk embeds in its uploads.

## How it works

1. Queries `GET /api/time` on the TLR server and records `t1` (before) and `t3` (after).
2. Uses NTP-style maths to compensate for network round-trip time:
   `offset = serverTime − (t1 + t3) / 2`
3. Calls the OS (`settimeofday` on Linux/macOS, `SetSystemTime` on Windows) to apply
   the corrected time.

## Requirements

- Must run as **root** (Linux/macOS) or **Administrator** (Windows) to set the system clock.
- The TLR server must be reachable over HTTP/HTTPS.

## Configuration

Edit `tlr-time-sync.ini` before running:

```ini
[server]
# Root URL of your TLR server (no trailing slash)
url = https://your-tlr-server.example.com

[sync]
# How often to sync, in seconds (minimum enforced: 10)
interval_seconds = 30

# Consecutive failures before exponential back-off kicks in
failure_threshold = 5
```

## Building

```bash
# Current platform
go build -o tlr-time-sync .

# Windows (from Linux/macOS)
GOOS=windows GOARCH=amd64 go build -o tlr-time-sync.exe .

# Linux (from macOS)
GOOS=linux GOARCH=amd64 go build -o tlr-time-sync-linux .
```

## Installing as a system service

### Windows — double-click install

1. Extract the zip so `tlr-time-sync.exe` and `tlr-time-sync.ini` are in the **same folder**
2. Edit `tlr-time-sync.ini` and set your TLR server URL
3. **Double-click `tlr-time-sync.exe`**

Windows will show a UAC prompt ("Allow this app to make changes?"). Click **Yes**.
A console window opens and shows:

```
--------------------------------------------------
  TLR Time Sync installed as a Windows service.
  It will start automatically at every boot.
--------------------------------------------------

  You can close this window.

Press Enter to close...
```

That's it. The service (`TLRTimeSync`) is now running and will start automatically on every boot. You never need to run the exe again.

**To remove the service**, open an Administrator command prompt and run:
```bat
tlr-time-sync.exe uninstall
```

### Linux (run as root)

```bash
sudo cp tlr-time-sync /usr/local/bin/
sudo mkdir -p /etc/tlr-time-sync
sudo cp tlr-time-sync.ini /etc/tlr-time-sync/
sudo tlr-time-sync install -config /etc/tlr-time-sync/tlr-time-sync.ini
```

Generates and enables a **systemd unit** that runs as root on boot.

```bash
sudo tlr-time-sync uninstall
```

### macOS (run as root)

```bash
sudo tlr-time-sync install -config /etc/tlr-time-sync.ini
```

Generates a **LaunchDaemon** plist and loads it — runs as root on boot.

## Running in the foreground (debug / testing)

```bash
# No install needed — runs in terminal and logs to stderr, Ctrl-C to stop
sudo ./tlr-time-sync -config tlr-time-sync.ini
```

## Rate limiting & back-off

The client enforces a **1-second minimum between requests** regardless of
`interval_seconds`.  After `failure_threshold` consecutive failures it enters
exponential back-off: 5 s, 10 s, 20 s, 40 s … capped at 10 minutes.
This prevents a broken or unreachable server from being hammered.
