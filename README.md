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

The binary manages its own service registration. Run these commands **once** with
elevated privileges — after that the service starts automatically at every boot
with no further prompts.

### Windows (run as Administrator)

```bat
tlr-time-sync.exe install -config C:\ProgramData\TLRTimeSync\tlr-time-sync.ini
```

Installs and immediately starts a **Windows Service** (`TLRTimeSync`) running as
`LocalSystem`, which has native rights to set the system clock — no UAC prompt
at runtime, ever.

```bat
tlr-time-sync.exe uninstall   # stop and remove
tlr-time-sync.exe stop        # stop without removing
tlr-time-sync.exe start       # restart after a stop
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

## Running in the foreground (debug)

```bash
# Foreground mode — logs go to stderr, Ctrl-C to stop
sudo ./tlr-time-sync -config tlr-time-sync.ini run

# Or just omit the command (same thing)
sudo ./tlr-time-sync -config tlr-time-sync.ini
```

## Rate limiting & back-off

The client enforces a **1-second minimum between requests** regardless of
`interval_seconds`.  After `failure_threshold` consecutive failures it enters
exponential back-off: 5 s, 10 s, 20 s, 40 s … capped at 10 minutes.
This prevents a broken or unreachable server from being hammered.
