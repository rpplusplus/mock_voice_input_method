# Mock Voice Input Method

Mock Voice Input Method is a small LAN-based voice typing experiment. A mobile client sends recognized text to a desktop daemon, and the daemon pastes that text into the currently focused app.

It is not a native input method yet. The current design intentionally uses clipboard paste as the smallest cross-app path that works well for Chinese text, emoji, and multi-line dictation.

[中文文档](docs/README.zh-CN.md) | [Design notes](docs/design-notes.zh-CN.md)

## What Works Today

- HTTP daemon written in Go.
- Android/Expo sender app.
- macOS paste action through a Swift helper.
- Linux/KDE Wayland paste action through `wl-copy` and `ydotool`.
- Bearer token authentication.
- GUI and terminal paste modes.
- macOS clipboard restore after paste.

## Architecture

```text
Android / Expo client
  -> POST text over LAN
  -> Go daemon validates token
  -> platform paste action
  -> focused desktop app receives text
```

Platform actions:

- **macOS**: Go runs `VoicePasteHelper`, which writes `NSPasteboard`, sends `Command+V` with `CGEvent`, then restores the previous pasteboard.
- **Linux/KDE Wayland**: Go runs `wl-copy` to write the Wayland clipboard, then `ydotool` to send `Ctrl+V`.

The client protocol is platform-neutral. `ctrl_v` means normal paste mode; `ctrl_shift_v` means terminal-style paste mode. The daemon maps those modes to the right shortcut for the current OS.

## Repository Layout

```text
.
├── main.go                         # HTTP daemon
├── paste.go                        # shared paste types and timing config
├── paste_darwin.go                 # macOS paste backend
├── paste_linux.go                  # Linux paste backend
├── paste_unsupported.go            # unsupported-platform fallback
├── macos-helper/                   # Swift helper for NSPasteboard + CGEvent
├── android-client/                 # Expo/React Native sender app
├── docs/                           # design notes and Chinese documentation
└── test-curl.sh                    # local API test helper
```

## API

### `GET /health`

Returns:

```text
ok
```

### `POST /type`

```http
POST /type
Authorization: Bearer <token>
Content-Type: text/plain; charset=utf-8
X-Voice-Paste-Chord: ctrl_v

Hello from voice input
```

Responses:

- `204 No Content`: text was accepted and paste action completed.
- `400 Bad Request`: text is empty or whitespace-only.
- `401 Unauthorized`: missing or invalid token.
- `413 Request Entity Too Large`: request body is larger than 1 MiB.
- `500 Internal Server Error`: platform paste action failed.

`X-Voice-Paste-Chord` is optional:

- `ctrl_v`: normal GUI paste mode.
- `ctrl_shift_v`: terminal-style paste mode.

Current mappings:

| Mode | macOS | Linux |
| --- | --- | --- |
| `ctrl_v` | `Command+V` | `Ctrl+V` |
| `ctrl_shift_v` | `Command+Shift+V` | `Ctrl+Shift+V` |

## Configuration

The daemon is configured with environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `VOICE_DAEMON_ADDR` | `0.0.0.0:47832` | HTTP listen address. |
| `VOICE_DAEMON_TOKEN` | required | Bearer token. The daemon refuses to start without it. |
| `VOICE_DAEMON_PASTE_DELAY_MS` | `120` | Delay between writing clipboard and sending paste shortcut. |
| `VOICE_DAEMON_RESTORE_DELAY_MS` | `250` | macOS-only delay before restoring the previous clipboard. |
| `VOICE_DAEMON_MACOS_HELPER` | auto-detected | Path to `VoicePasteHelper` on macOS. |

There is no tokenless mode. A LAN device that knows the token can type into the focused desktop app, so keep the token private.

## Quick Start

### macOS

Build the Swift helper:

```bash
cd macos-helper
swift build -c release
cd ..
```

Start the daemon:

```bash
export VOICE_DAEMON_TOKEN=test-token
export VOICE_DAEMON_ADDR=127.0.0.1:47832
go run .
```

macOS must allow the helper, or the terminal that launches it, to control the computer:

```text
System Settings -> Privacy & Security -> Accessibility
```

If the helper is not in the default build location, set:

```bash
export VOICE_DAEMON_MACOS_HELPER=/path/to/VoicePasteHelper
```

### Linux/KDE Wayland

Install the runtime tools:

- `wl-copy` from `wl-clipboard`
- `ydotool`
- `/dev/uinput` access, usually through `ydotoold`

Start `ydotoold` with your distro's recommended setup. On systems with the user service:

```bash
systemctl --user enable --now ydotool.service
```

Start the daemon:

```bash
export VOICE_DAEMON_TOKEN=test-token
export VOICE_DAEMON_ADDR=127.0.0.1:47832
go run .
```

### Test With curl

Send text from another terminal:

```bash
VOICE_DAEMON_TOKEN=test-token ./test-curl.sh 'Hello, desktop'
```

If you need time to focus a target input field first:

```bash
sleep 3; VOICE_DAEMON_TOKEN=test-token ./test-curl.sh 'Hello after focus'
```

To test from another device on the same LAN, bind the daemon to `0.0.0.0:47832` and send:

```bash
curl -X POST \
  -H "Authorization: Bearer <token>" \
  --data-binary 'Text from another device' \
  http://<desktop-lan-ip>:47832/type
```

## Android Client

The sender app lives in `android-client/`.

```bash
cd android-client
npm install
npm run android
```

Configure the daemon URL, token, and paste mode in the app. The client does not need to know whether the desktop is macOS, Linux, or a future Windows backend.

## Development

Run Go tests:

```bash
go test ./...
```

Build the macOS helper:

```bash
cd macos-helper
swift build -c release
```

## Security and Privacy

- The daemon accepts text over HTTP from the network address you bind.
- A valid token allows a client to paste into the currently focused desktop app.
- The token should be random and private.
- macOS restores the previous clipboard after paste.
- Linux currently uses `wl-copy --sensitive`, but clipboard managers may still keep history depending on desktop behavior.

## Status

This is an early experiment that has been tested manually on macOS and Linux/KDE Wayland. The protocol and code layout are intentionally small so new platform paste backends can be added without changing the mobile client.

Windows support is not implemented yet.
