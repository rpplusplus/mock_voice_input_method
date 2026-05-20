# Mock Voice Input Method

一个局域网语音输入实验：手机端负责录入/发送文本，桌面端 daemon 负责把文本粘贴到当前焦点窗口。

当前实现不是系统输入法插件，而是一个“远程粘贴”闭环。这样可以先把中文、emoji、多行文本这些语音识别结果稳定送进任意 App，再决定后续是否需要接入平台原生输入法框架。

## 当前闭环

```text
Android / Expo 客户端
  -> HTTP POST 文本
  -> Go daemon 校验 token
  -> 按平台执行 paste action
  -> 当前焦点窗口收到文本
```

平台 action 现在有两条：

- macOS：Go daemon 调用 Swift helper，helper 写 `NSPasteboard`、发送 `Command+V`，随后恢复原剪贴板。
- Linux/KDE Wayland：Go daemon 调用 `wl-copy` 写剪贴板，再用 `ydotool` 模拟 `Ctrl+V`。

客户端协议保持平台无关。客户端发的 `ctrl_v` / `ctrl_shift_v` 表示“普通粘贴/终端粘贴模式”，桌面端按当前平台映射成实际快捷键。

## 项目结构

```text
.
├── main.go                         # HTTP daemon、token 校验、请求处理
├── paste.go                        # 通用 paste 类型、延迟配置、命令 helper
├── paste_darwin.go                 # macOS paste action，调用 Swift helper
├── paste_linux.go                  # Linux paste action，调用 wl-copy/ydotool
├── paste_unsupported.go            # 未支持平台的明确错误
├── macos-helper/
│   ├── Package.swift
│   └── Sources/VoicePasteHelper/
│       └── main.swift              # NSPasteboard + CGEvent helper
├── android-client/                 # Expo/React Native 发送端
└── test-curl.sh                    # 本机 curl 测试脚本
```

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

macOS 下当前映射为：

- `ctrl_v` -> `Command+V`
- `ctrl_shift_v` -> `Command+Shift+V`

Linux 下当前映射为：

- `ctrl_v` -> `Ctrl+V`
- `ctrl_shift_v` -> `Ctrl+Shift+V`

## 配置

daemon 使用环境变量配置：

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

## macOS

macOS 路线：

```text
POST /type
  -> Go daemon
  -> exec Swift helper，stdin 传 JSON
  -> helper 保存当前剪贴板
  -> helper 写 NSPasteboard
  -> helper 发送 Command+V / Command+Shift+V
  -> helper 恢复原剪贴板
```

先构建 helper：

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

如果 helper 不在默认位置：

```bash
export VOICE_DAEMON_MACOS_HELPER=/path/to/VoicePasteHelper
```

macOS 必须给 helper 或承载它的终端授予辅助功能权限，否则只能写剪贴板，不能发按键：

```text
System Settings -> Privacy & Security -> Accessibility
```

### 剪贴板恢复

helper 会尽量保存原 pasteboard 的每个 item、type 和 data，粘贴后再恢复。这样语音文本不会长期覆盖系统剪贴板。

如果某些 App 偶尔粘贴不到，通常是恢复太快，目标 App 还没读取剪贴板。可以调大恢复延迟：

```bash
export VOICE_DAEMON_RESTORE_DELAY_MS=500
```

## Linux/KDE Wayland

Linux 路线：

```text
POST /type
  -> Go daemon
  -> wl-copy 写 Wayland 剪贴板
  -> ydotool 模拟 Ctrl+V / Ctrl+Shift+V
```

依赖：

- Go
- Wayland 会话
- `wl-copy`，通常来自 `wl-clipboard`
- `ydotool`
- `/dev/uinput` 权限，或运行 `ydotoold`

等价命令：

```bash
printf '%s' "$TEXT" | wl-copy --type 'text/plain;charset=utf-8' --sensitive
sleep 0.12
ydotool key --key-delay 20 29:1 47:1 47:0 29:0
```

终端模式：

```bash
ydotool key --key-delay 20 29:1 42:1 47:1 47:0 42:0 29:0
```

如果用户服务可用，推荐启动 `ydotool.service`：

```bash
systemctl --user enable --now ydotool.service
```

临时方式可以直接启动 `ydotoold`，并把 socket 暴露给当前用户：

```bash
sudo ydotoold \
  --socket-path=/run/user/1000/.ydotool_socket \
  --socket-perm=0600 \
  --socket-own=1000:1000
```

### KDE/Klipper 历史

`wl-copy --sensitive` 会尽量提示剪贴板管理器这段内容敏感，但不同剪贴板管理器是否完全尊重这个 hint 需要实测。

KDE 的当前剪贴板和 Klipper 历史不是同一个东西。即使后续清空或恢复当前剪贴板，也不一定能删除已经进入 Klipper 历史的记录。

如果 `wl-copy --sensitive` 不够，下一步可以实现 KDE 感知剪贴板写入，同时提供：

```text
text/plain;charset=utf-8 = <voice text>
x-kde-passwordManagerHint = secret
```

## Android/Expo 客户端

客户端在 `android-client/` 下，是一个 Expo/React Native 发送器。它目前不需要知道桌面端是 macOS、Linux 还是未来的 Windows。

客户端需要配置：

- daemon URL，例如 `http://192.168.1.100:47832`
- `VOICE_DAEMON_TOKEN`
- 粘贴模式：GUI / Terminal

运行：

```bash
cd android-client
npm install
npm run android
```

## 测试

### Go 单元测试

```bash
go test ./...
```

### macOS helper 构建

```bash
cd macos-helper
swift build -c release
```

### 本机 curl 测试

启动 daemon：

```bash
export VOICE_DAEMON_TOKEN=test-token
export VOICE_DAEMON_ADDR=127.0.0.1:47832
go run .
```

另一个终端发送：

```bash
VOICE_DAEMON_TOKEN=test-token ./test-curl.sh '你好，世界'
```

如果需要先把焦点切到目标输入框，可以延迟发送：

```bash
sleep 3; VOICE_DAEMON_TOKEN=test-token ./test-curl.sh '你好，macOS'
```

### 局域网测试

从手机或另一台设备发送：

```bash
curl -X POST \
  -H "Authorization: Bearer <token>" \
  --data-binary '来自手机的中文语音文本' \
  http://<desktop-lan-ip>:47832/type
```

预期：

- 当前焦点窗口收到文本。
- token 错误时返回 `401`。

建议覆盖：

- 中文
- 英文
- emoji
- 多行文本
- 长文本
- 空文本
- 普通输入框
- Terminal/iTerm

## systemd user service 示例

Linux 上后续可以用 systemd user service 随用户登录启动：

```ini
[Unit]
Description=Mock Voice Input Daemon

[Service]
ExecStart=/home/txx/workspace/mock_voice_input_method/mock-voice-daemon
Environment=VOICE_DAEMON_ADDR=0.0.0.0:47832
Environment=VOICE_DAEMON_TOKEN=replace-with-random-token
Restart=on-failure

[Install]
WantedBy=default.target
```

token 不建议写进公开仓库。个人机器上可以放在用户私有的 environment file，或由安装脚本生成。

## 后续路线

- macOS：如果每次 `exec` helper 的方式不够理想，可以升级为常驻 helper，通过 Unix domain socket 或 XPC 与 Go daemon 通信。
- macOS：如果裸 CLI 的辅助功能授权体验不稳定，可以把 helper 包成 `.app` 或 LaunchAgent。
- Linux/KDE：继续实测 Klipper 历史污染，必要时实现 KDE MIME hint 的原生剪贴板写入。
- Windows：保留现有 HTTP 协议，新增 Windows paste action，可用 Win32 clipboard + `SendInput` 实现。
- 客户端：协议稳定后再考虑二维码配对、自动发现、token 管理和更完整的语音识别体验。
