# iMessage Setup (macOS Only)

FractalBot supports native iMessage on macOS. Inbound messages are polled from the Messages database (`chat.db`); outbound replies are sent via AppleScript.

## Prerequisites

- **macOS** — iMessage channel is darwin-only.
- **Messages app** signed in with an Apple ID.
- **Full Disk Access (FDA)** granted to the FractalBot `.app` bundle.

## 1) Create the .app Bundle

macOS TCC requires an `.app` bundle for FDA grants. Place the FractalBot binary inside a minimal bundle:

```
/Applications/FractalBot.app/
├── Contents/
│   ├── Info.plist
│   └── MacOS/
│       └── FractalBot          # Go binary (not a wrapper script)
```

**Info.plist:**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>FractalBot</string>
  <key>CFBundleIdentifier</key>
  <string>ai.fractalmind.fractalbot</string>
  <key>CFBundleName</key>
  <string>FractalBot</string>
  <key>CFBundleVersion</key>
  <string>1.0</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>LSUIElement</key>
  <true/>
</dict>
</plist>
```

Build and install:

```bash
go build -o /Applications/FractalBot.app/Contents/MacOS/FractalBot ./cmd/fractalbot
codesign --force --deep --sign - --identifier ai.fractalmind.fractalbot /Applications/FractalBot.app
```

> **Important:** The Go binary must be the direct executable in the `.app` bundle — not a bash wrapper that `exec`s a separate binary. TCC validates the code signature of the actual running process.

## 2) Grant Full Disk Access

FractalBot reads `~/Library/Messages/chat.db` using an embedded SQLite driver (`modernc.org/sqlite`). macOS TCC blocks this unless FDA is granted.

1. Open **System Settings → Privacy & Security → Full Disk Access**.
2. Click **+** and add `/Applications/FractalBot.app`.
3. Ensure the toggle is **on**.

> **Note:** Rebuilding the binary changes the code signature hash. After each rebuild + deploy, you must **remove and re-add** the `.app` in FDA settings.

## 3) Configure FractalBot

```yaml
channels:
  imessage:
    enabled: true
    recipient: "+1234567890"        # default outbound target (phone/email/Apple ID)
    pollingEnabled: true            # enable inbound message polling
    pollingIntervalSeconds: 5       # poll interval (min: 1, default: 5)
    pollingLimit: 20                # max messages per poll (max: 100, default: 20)
    # databasePath: "~/Library/Messages/chat.db"  # override if needed
    # service: ""                   # leave empty for auto-discovery
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable iMessage channel |
| `recipient` | (required) | Default outbound recipient |
| `pollingEnabled` | `false` | Enable inbound polling from chat.db |
| `pollingIntervalSeconds` | `5` | Seconds between polls |
| `pollingLimit` | `20` | Max messages fetched per poll |
| `databasePath` | `~/Library/Messages/chat.db` | Messages database path |
| `service` | (auto) | iMessage service identifier (auto-discovered at startup) |

## 4) Run as LaunchAgent

Create `~/Library/LaunchAgents/ai.fractalmind.fractalbot.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>ai.fractalmind.fractalbot</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Applications/FractalBot.app/Contents/MacOS/FractalBot</string>
    <string>--config</string>
    <string>/Users/YOU/.config/fractalbot/config.yaml</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/Users/YOU/Library/Logs/fractalbot/stdout.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/YOU/Library/Logs/fractalbot/stderr.log</string>
  <key>ProcessType</key>
  <string>Background</string>
</dict>
</plist>
```

Load and start:

```bash
mkdir -p ~/Library/Logs/fractalbot
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/ai.fractalmind.fractalbot.plist
```

Restart after rebuild:

```bash
launchctl kickstart -k gui/$(id -u)/ai.fractalmind.fractalbot
```

## 5) Verify

Check logs for successful startup:

```bash
tail -20 ~/Library/Logs/fractalbot/stderr.log
```

Expected output:

```
imessage: resolved service ID: E4F50C86-...
imessage: initialized lastSeenMessageID to 291
```

Send a test iMessage from your phone to the Mac. The log should show polling activity. Replies are sent back via AppleScript using the auto-discovered service UUID.

## Architecture

```
iPhone ──iMessage──▶ Messages.app ──▶ ~/Library/Messages/chat.db
                                            │
                                    FractalBot polls (embedded SQLite)
                                            │
                                            ▼
                                    ohMyCode / agent-manager
                                            │
                                            ▼
                                    FractalBot sends reply
                                            │
                                    osascript (AppleScript)
                                            │
                                            ▼
                              Messages.app ──iMessage──▶ iPhone
```

**Inbound:** Embedded SQLite (`modernc.org/sqlite`) reads `chat.db` directly in-process. This avoids spawning `sqlite3` subprocesses which cannot inherit FDA from the parent `.app` bundle.

**Outbound:** AppleScript sends via `buddy` lookup using the auto-discovered iMessage service UUID. The legacy `"E:iMessage"` service name no longer works on newer macOS versions.

**Startup:**
1. Resolve iMessage service UUID via AppleScript (best-effort; outbound degrades gracefully if it fails).
2. If polling enabled: verify FDA, ensure Messages.app is running, initialize `lastSeenMessageID` to current max inbound ROWID (prevents replaying history after restart).

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `unable to open database file: out of memory (14)` | FDA not granted or invalidated by rebuild | Remove + re-add `.app` in FDA settings |
| `failed to resolve service ID` | Messages.app not running or no iMessage account | Sign in to Messages, ensure iMessage is active |
| `Invalid key form (-10002)` | Outdated service name (`E:iMessage`) | Update to latest code with auto UUID discovery |
| `Messages app did not start in time` | Messages.app crashed or not installed | Open Messages.app manually, check account |
| No inbound messages detected | `pollingEnabled: false` or wrong `databasePath` | Set `pollingEnabled: true` in config |

## Safety

- FDA grants read access to all Messages history. Keep the `.app` bundle secure.
- DM-only: inbound polling filters `is_from_me = 0` (only messages from others).
- No public ports required. All communication is local (Messages.app ↔ chat.db ↔ FractalBot).
