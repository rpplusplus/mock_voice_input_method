# Mock Voice Input Method 中文文档

这是一个局域网语音输入实验：手机端负责录入或发送文本，桌面端 daemon 负责把文本粘贴到当前焦点窗口。

当前实现不是系统输入法插件，而是一个“远程粘贴”闭环。这样可以先把中文、emoji、多行文本这些语音识别结果稳定送进任意 App，再决定后续是否接入平台原生输入法框架。

更详细的设计背景见 [design-notes.zh-CN.md](design-notes.zh-CN.md)。

## 当前闭环

```text
Android / Expo 客户端
  -> HTTP POST 文本
  -> Go daemon 校验 token
  -> 按平台执行 paste action
  -> 当前焦点窗口收到文本
```

平台 action：

- macOS：Go daemon 调用 Swift helper，helper 写 `NSPasteboard`、发送 `Command+V`，随后恢复原剪贴板。
- Linux/KDE Wayland：Go daemon 调用 `wl-copy` 写剪贴板，再用 `ydotool` 模拟 `Ctrl+V`。

客户端协议保持平台无关。客户端发的 `ctrl_v` / `ctrl_shift_v` 表示“普通粘贴/终端粘贴模式”，桌面端按当前平台映射成实际快捷键。

## HTTP 接口

### `GET /health`

返回：

```text
ok
```

### `POST /type`

```http
POST /type
Authorization: Bearer <token>
Content-Type: text/plain; charset=utf-8
X-Voice-Paste-Chord: ctrl_v

你好，世界
```

行为：

- 请求体就是要插入的文本。
- token 正确时返回 `204 No Content`。
- token 缺失或错误时返回 `401 Unauthorized`。
- 文本为空或全空白时返回 `400 Bad Request`。
- 请求体超过 1 MiB 时返回 `413 Request Entity Too Large`。
- 粘贴失败时返回 `500 Internal Server Error`，daemon 日志会记录具体错误。

`X-Voice-Paste-Chord` 可选：

- `ctrl_v`：普通 GUI 输入框。
- `ctrl_shift_v`：终端类目标，或需要 Shift 粘贴的目标。

当前映射：

| 模式 | macOS | Linux |
| --- | --- | --- |
| `ctrl_v` | `Command+V` | `Ctrl+V` |
| `ctrl_shift_v` | `Command+Shift+V` | `Ctrl+Shift+V` |

## 配置

```text
VOICE_DAEMON_ADDR=0.0.0.0:47832
VOICE_DAEMON_TOKEN=<random-token>
VOICE_DAEMON_PASTE_DELAY_MS=120
VOICE_DAEMON_RESTORE_DELAY_MS=250
VOICE_DAEMON_MACOS_HELPER=/path/to/VoicePasteHelper
```

说明：

- `VOICE_DAEMON_ADDR` 默认 `0.0.0.0:47832`。
- `VOICE_DAEMON_TOKEN` 没有默认值；未设置时 daemon 启动失败。
- `VOICE_DAEMON_PASTE_DELAY_MS` 默认 `120`，写入剪贴板后等待多久再触发粘贴。
- `VOICE_DAEMON_RESTORE_DELAY_MS` 默认 `250`，macOS 下触发粘贴后等待多久再恢复原剪贴板。
- `VOICE_DAEMON_MACOS_HELPER` 只在 macOS 下使用。未设置时优先找 `macos-helper/.build/release/VoicePasteHelper`，再找 daemon 同目录下的 `VoicePasteHelper`，最后回退到 `PATH`。

不提供无 token 模式，避免局域网内任意设备都能向当前窗口输入内容。

## macOS 快速开始

构建 helper：

```bash
cd macos-helper
swift build -c release
cd ..
```

启动 daemon：

```bash
export VOICE_DAEMON_TOKEN=test-token
export VOICE_DAEMON_ADDR=127.0.0.1:47832
go run .
```

macOS 必须给 helper 或承载它的终端授予辅助功能权限，否则只能写剪贴板，不能发按键：

```text
System Settings -> Privacy & Security -> Accessibility
```

如果 helper 不在默认位置：

```bash
export VOICE_DAEMON_MACOS_HELPER=/path/to/VoicePasteHelper
```

### 剪贴板恢复

helper 会尽量保存原 pasteboard 的每个 item、type 和 data，粘贴后再恢复。这样语音文本不会长期覆盖系统剪贴板。

如果某些 App 偶尔粘贴不到，通常是恢复太快，目标 App 还没读取剪贴板。可以调大恢复延迟：

```bash
export VOICE_DAEMON_RESTORE_DELAY_MS=500
```

## Linux/KDE Wayland 快速开始

依赖：

- Go
- Wayland 会话
- `wl-copy`，通常来自 `wl-clipboard`
- `ydotool`
- `/dev/uinput` 权限，或运行 `ydotoold`

如果用户服务可用，推荐启动 `ydotool.service`：

```bash
systemctl --user enable --now ydotool.service
```

启动 daemon：

```bash
export VOICE_DAEMON_TOKEN=test-token
export VOICE_DAEMON_ADDR=127.0.0.1:47832
go run .
```

## Android/Expo 客户端

客户端在 `android-client/` 下，是一个 Expo/React Native 发送器。它目前不需要知道桌面端是 macOS、Linux 还是未来的 Windows。

运行：

```bash
cd android-client
npm install
npm run android
```

客户端需要配置：

- daemon URL，例如 `http://192.168.1.100:47832`
- `VOICE_DAEMON_TOKEN`
- 粘贴模式：GUI / Terminal

## 测试

Go 单元测试：

```bash
go test ./...
```

macOS helper 构建：

```bash
cd macos-helper
swift build -c release
```

本机 curl 测试：

```bash
VOICE_DAEMON_TOKEN=test-token ./test-curl.sh '你好，世界'
```

如果需要先把焦点切到目标输入框，可以延迟发送：

```bash
sleep 3; VOICE_DAEMON_TOKEN=test-token ./test-curl.sh '你好，macOS'
```

局域网测试：

```bash
curl -X POST \
  -H "Authorization: Bearer <token>" \
  --data-binary '来自手机的中文语音文本' \
  http://<desktop-lan-ip>:47832/type
```
