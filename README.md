# Linux/KDE Wayland 语音输入 Daemon 设计

## 目标

这个项目先验证一个很小的闭环：

```text
iPhone / Expo 语音识别
  -> 局域网 POST 文本
  -> Linux Go daemon
  -> 写入 Wayland 剪贴板
  -> 模拟 Ctrl+V
  -> 当前焦点窗口收到文本
```

当前阶段只考虑这台 Linux/KDE Wayland 机器，不考虑 macOS、Windows、X11、托盘 UI、二维码配对、自动发现或输入法插件。

## 为什么先用剪贴板

语音识别结果天然是一段 Unicode 字符串，尤其是中文、emoji、多行文本。Wayland 下普通进程没有一个通用的 `InsertText("...")` API 可以直接把字符串提交给当前焦点控件。

实际可行路径主要是：

- 剪贴板粘贴：最适合中文成段文本，最容易跑通。
- 虚拟键盘：适合按键和英文，不适合直接提交中文字符串。
- 输入法框架：概念上最正确，但需要接 Fcitx5/IBus，复杂度明显更高。
- RemoteDesktop/EIS/portal：更符合 Wayland 权限模型，但本质仍是输入事件，不是字符串提交接口。

所以 v0 选择剪贴板粘贴，把复杂度压到最低。

## v0 架构

### HTTP 接口

daemon 提供一个接口：

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
- 文本为空时返回 `400 Bad Request`。
- 粘贴失败时返回 `500 Internal Server Error`，并记录具体错误。
- `X-Voice-Paste-Chord` 可选，支持 `ctrl_v` 和 `ctrl_shift_v`。普通 GUI 输入框用 `ctrl_v`，Konsole/终端/Codex 这类目标通常用 `ctrl_shift_v`。

### 配置

使用环境变量配置：

```text
VOICE_DAEMON_ADDR=0.0.0.0:47832
VOICE_DAEMON_TOKEN=<random-token>
VOICE_DAEMON_PASTE_DELAY_MS=120
```

默认值：

- `VOICE_DAEMON_ADDR` 默认 `0.0.0.0:47832`。
- `VOICE_DAEMON_TOKEN` 没有默认值；未设置时 daemon 启动失败。
- `VOICE_DAEMON_PASTE_DELAY_MS` 默认 `120`，用于等待 `wl-copy` 的剪贴板内容就绪后再触发粘贴。

不提供无 token 模式，避免局域网内任意设备都能向当前窗口输入内容。

### MVP 粘贴流程

第一版直接 shell out 到系统工具：

```text
1. 收到 POST /type
2. 校验 Authorization token
3. 调用 wl-copy，把请求体写入 Wayland 剪贴板，并加敏感内容 hint
4. 调用 ydotool，模拟 Ctrl+V
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

## KDE 剪贴板历史问题

KDE 的当前剪贴板和 Klipper 历史不是同一个东西。

直接使用普通 `wl-copy` 会改变当前剪贴板，也可能被 Klipper 保存到历史。之后清空或恢复剪贴板，只能影响当前剪贴板，不一定能删除已经进入 Klipper 历史的记录。

这意味着 MVP 路线有一个明确副作用：

```text
语音输入文本可能出现在 Klipper 历史里
```

v0 会使用 `wl-copy --sensitive` 尽量提示剪贴板管理器这是一段敏感内容。这个选项比裸 `wl-copy` 更好，但仍然需要在 KDE/Klipper 上实测确认是否完全不进历史。

## KDE 感知路线

如果 MVP 可用，但 `wl-copy --sensitive` 仍然不能满足 Klipper 历史控制，下一步改成 daemon 自己设置 MIME data 的 KDE 感知剪贴板写入。

思路是：daemon 不再只调用 `wl-copy`，而是自己设置 clipboard MIME data，同时提供普通文本和 KDE/Klipper 可识别的敏感内容 hint。

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

## 运行依赖

MVP 路线需要：

- Go
- Wayland 会话
- `wl-copy`，通常来自 `wl-clipboard`
- `ydotool`
- `/dev/uinput` 权限，或运行 `ydotoold`

当前机器已确认：

- 会话类型是 Wayland。
- Go 可用。
- systemd 可用。
- `wl-copy` 已安装。
- `ydotool` 已安装。
- 用户 `txx` 已加入 `input` 组；需要重新登录后，`ydotool.service` 才能用新的组权限访问 `/dev/uinput`。

如果不想重新登录，也可以临时用 root 启动 ydotoold，并把 socket 暴露给当前用户：

```bash
sudo ydotoold \
  --socket-path=/run/user/1000/.ydotool_socket \
  --socket-perm=0600 \
  --socket-own=1000:1000
```

重新登录后，推荐使用用户服务：

```bash
systemctl --user enable --now ydotool.service
```

## systemd user service 形态

后续实现 daemon 后，推荐使用 systemd user service 随用户登录启动。

示例形态：

```ini
[Unit]
Description=Mock Voice Input Daemon

[Service]
ExecStart=/home/txx/workspace/mock_voice_input_methods/mock-voice-daemon
Environment=VOICE_DAEMON_ADDR=0.0.0.0:47832
Environment=VOICE_DAEMON_TOKEN=replace-with-random-token
Restart=on-failure

[Install]
WantedBy=default.target
```

token 不建议长期直接写在公开仓库里的 service 文件中。个人机器上可以放在用户私有的 environment file，或由安装脚本生成。

## 测试计划

### 本机测试

先把焦点放在 KDE 文本编辑器、浏览器输入框或终端里，然后执行：

```bash
export VOICE_DAEMON_TOKEN=test-token
export VOICE_DAEMON_ADDR=127.0.0.1:47832

go build -o mock-voice-daemon .
./mock-voice-daemon
```

另开一个终端执行：

```bash
curl -X POST \
  -H "Authorization: Bearer $VOICE_DAEMON_TOKEN" \
  --data-binary '你好，世界' \
  http://127.0.0.1:47832/type
```

也可以使用仓库里的测试脚本：

```bash
./test-curl.sh '你好，世界'
```

预期：

- 当前焦点窗口收到 `你好，世界`。
- HTTP 返回 `204`。

### 局域网测试

从手机或另一台设备发送：

```bash
curl -X POST \
  -H "Authorization: Bearer <token>" \
  --data-binary '来自手机的中文语音文本' \
  http://<pc-lan-ip>:47832/type
```

预期：

- PC 当前焦点窗口收到文本。
- token 错误时返回 `401`。

### 文本类型测试

需要覆盖：

- 中文
- 英文
- emoji
- 多行文本
- 长文本
- 空文本

### Klipper 历史测试

MVP 路线：

- 发送一段测试文本。
- 打开 Klipper 历史。
- 记录该文本是否进入历史。

KDE 感知路线：

- 使用带 `x-kde-passwordManagerHint=secret` 的 clipboard 写入实现。
- 重复发送测试文本。
- 检查 Klipper 是否跳过历史记录。

## 迭代顺序

1. 实现 Go HTTP daemon 和 token 校验。
2. 接入 `wl-copy` 和 `ydotool`，完成 MVP 粘贴。
3. 添加基础日志和错误返回。
4. 增加 systemd user service。
5. 实测 Klipper 历史污染。
6. 如有必要，改造为 KDE 感知剪贴板写入。
