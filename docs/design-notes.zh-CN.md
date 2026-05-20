# 设计笔记

这份文档保存项目早期讨论、平台实现细节和后续路线。根目录 README 只保留开源项目首页需要的信息。

## 为什么先用剪贴板

语音识别结果天然是一段 Unicode 字符串，尤其是中文、emoji、多行文本。桌面平台通常没有一个对所有 App 都可用的 `InsertText("...")` API。

实际可行路径主要是：

- 剪贴板粘贴：最适合中文成段文本，最容易跨 App 跑通。
- 虚拟键盘：适合按键和英文，不适合直接提交中文字符串。
- 输入法框架：概念上最正确，但平台差异大，启动复杂度明显更高。
- 远程桌面/辅助功能/portal：更符合部分平台权限模型，但本质仍然更接近输入事件，不是字符串提交接口。

所以当前版本选择剪贴板粘贴，把复杂度压到最低。

## 平台 action 边界

客户端和 HTTP 协议不应该关心桌面端操作系统。Go daemon 负责统一处理：

- HTTP server
- token 校验
- 请求体大小限制
- 空文本拒绝
- paste chord 解析
- 日志和错误返回

平台差异只放在 paste action：

```text
paste_linux.go       -> wl-copy + ydotool
paste_darwin.go      -> Swift helper
paste_unsupported.go -> 明确返回未实现
```

这样未来增加 Windows 时，只需要新增 `paste_windows.go`，客户端可以不动。

## macOS 路线

当前 macOS 路线：

```text
POST /type
  -> Go daemon
  -> exec Swift helper，stdin 传 JSON
  -> helper 保存当前剪贴板
  -> helper 写 NSPasteboard
  -> helper 发送 Command+V / Command+Shift+V
  -> helper 恢复原剪贴板
```

Go 传给 helper 的 JSON 形态：

```json
{
  "text": "你好，世界",
  "chord": "ctrl_v",
  "delay_ms": 120,
  "restore_delay_ms": 250
}
```

### 为什么不用 AppleScript

AppleScript 可以快速验证：

```text
tell application "System Events" to keystroke "v" using command down
```

但它不适合作为长期方案：

- 依赖 `System Events`。
- 错误边界不清晰。
- 权限体验和调试都不够直接。
- 后续要做常驻 helper、LaunchAgent 或 app bundle 时迁移成本更高。

Swift helper 可以直接使用 `NSPasteboard` 和 `CGEvent`，后续更容易演进。

### 为什么 helper 先不是常驻进程

当前请求频率很低，每次 paste 都 `exec` helper 成本可以接受。这个形态有几个好处：

- 安装和调试简单。
- helper 逻辑边界清晰。
- Go daemon 不需要维护本地 socket 生命周期。
- 出错可以直接通过 stderr 返回给 daemon。

如果后续频率更高，或需要更稳定的权限体验，可以升级为：

- Unix domain socket 常驻 helper。
- XPC service。
- `.app` 包或 LaunchAgent。

### macOS 辅助功能权限

`CGEvent` 模拟按键需要 Accessibility 权限。实际测试中，可能需要授权 helper 二进制、Terminal/iTerm，或未来的 app bundle。裸 CLI 的 TCC 体验不一定稳定，所以后续可能需要包装成 `.app`。

### macOS 剪贴板恢复

helper 当前会读取 `NSPasteboard.general` 的所有 items、types 和 data，粘贴后再写回。这样可以避免语音文本长期覆盖用户原剪贴板。

恢复延迟由 `VOICE_DAEMON_RESTORE_DELAY_MS` 控制，默认 `250`。如果目标 App 偶尔粘贴不到，通常是恢复太快，目标 App 还没读取剪贴板，可以调大到 `500` 或更高。

注意：这只能恢复当前剪贴板，不保证第三方剪贴板管理器不会记录短暂出现过的语音文本。

## Linux/KDE Wayland 路线

Linux MVP 直接 shell out 到系统工具：

```text
1. 收到 POST /type
2. 校验 Authorization token
3. 调用 wl-copy，把请求体写入 Wayland 剪贴板，并加 sensitive hint
4. 调用 ydotool，模拟 Ctrl+V 或 Ctrl+Shift+V
5. 返回 204
```

等价命令：

```bash
printf '%s' "$TEXT" | wl-copy --type 'text/plain;charset=utf-8' --sensitive
sleep 0.12
ydotool key --key-delay 20 29:1 47:1 47:0 29:0
```

其中：

- `29:1` 是 Ctrl down
- `47:1` 是 V down
- `47:0` 是 V up
- `29:0` 是 Ctrl up
- `--key-delay 20` 让按键事件之间间隔 20ms，比一次性发送更稳

终端模式使用 `Ctrl+Shift+V`：

```bash
ydotool key --key-delay 20 29:1 42:1 47:1 47:0 42:0 29:0
```

如果不想重新登录以刷新 `/dev/uinput` 权限，可以临时用 root 启动 `ydotoold`，并把 socket 暴露给当前用户：

```bash
sudo ydotoold \
  --socket-path=/run/user/1000/.ydotool_socket \
  --socket-perm=0600 \
  --socket-own=1000:1000
```

重新登录后，更推荐用户服务：

```bash
systemctl --user enable --now ydotool.service
```

## KDE/Klipper 历史问题

KDE 的当前剪贴板和 Klipper 历史不是同一个东西。

直接使用普通 `wl-copy` 会改变当前剪贴板，也可能被 Klipper 保存到历史。之后清空或恢复剪贴板，只能影响当前剪贴板，不一定能删除已经进入 Klipper 历史的记录。

这意味着 Linux MVP 路线有一个明确副作用：

```text
语音输入文本可能出现在 Klipper 历史里
```

当前使用 `wl-copy --sensitive` 尽量提示剪贴板管理器这是一段敏感内容。这个选项比裸 `wl-copy` 更好，但仍然需要在 KDE/Klipper 上实测确认是否完全不进历史。

如果 MVP 可用，但 `wl-copy --sensitive` 仍然不能满足 Klipper 历史控制，下一步可以改成 daemon 自己设置 MIME data 的 KDE 感知剪贴板写入。

目标 MIME data：

```text
text/plain;charset=utf-8 = <voice text>
x-kde-passwordManagerHint = secret
```

预期效果：

- 当前焦点窗口仍然可以通过 `Ctrl+V` 粘贴普通文本。
- Klipper 看到 `x-kde-passwordManagerHint=secret` 后，应尽量避免把这条内容保存进历史。

注意：

- 这是 KDE/Klipper 约定，不是通用 Linux 剪贴板标准。
- 不保证所有剪贴板管理器都会尊重这个 hint。
- 当前 `wl-copy --sensitive` 已经能表达敏感内容 hint；如果它在 KDE 实测不够，再考虑自己实现 clipboard MIME data。

## systemd user service 形态

Linux 上后续可以用 systemd user service 随用户登录启动。

示例：

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

## 测试建议

建议覆盖：

- 中文
- 英文
- emoji
- 多行文本
- 长文本
- 空文本
- 普通输入框
- Terminal/iTerm
- token 错误
- daemon 监听 `127.0.0.1` 和 `0.0.0.0`
- macOS 剪贴板恢复
- KDE/Klipper 历史是否污染

## 后续路线

- macOS：如果每次 `exec` helper 的方式不够理想，可以升级为常驻 helper，通过 Unix domain socket 或 XPC 与 Go daemon 通信。
- macOS：如果裸 CLI 的辅助功能授权体验不稳定，可以把 helper 包成 `.app` 或 LaunchAgent。
- Linux/KDE：继续实测 Klipper 历史污染，必要时实现 KDE MIME hint 的原生剪贴板写入。
- Windows：保留现有 HTTP 协议，新增 Windows paste action，可用 Win32 clipboard + `SendInput` 实现。
- 客户端：协议稳定后再考虑二维码配对、自动发现、token 管理和更完整的语音识别体验。
