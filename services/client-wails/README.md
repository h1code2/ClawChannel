# ClawChannel Wails Client

面向 macOS 的桌面聊天客户端（Telegram Desktop 风格）。

## 特性（首版）
- 左侧会话列表 + 右侧聊天区
- 深色主题、气泡消息、圆角输入区
- 输入框右侧发送按钮（Enter 发送，Shift+Enter 换行）
- 设置面板独立（Gateway URL / Token / Agent）
- WebSocket 长连接、自动重连、ACK 回执
- 本地会话持久化（localStorage）

## 目录
- `main.go` / `app.go`: Wails 启动入口
- `frontend/dist/index.html`
- `frontend/dist/styles.css`
- `frontend/dist/app.js`

## 运行（开发机）
需要先安装 Wails CLI：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

然后在本目录执行：

```bash
wails dev
```

## 构建

```bash
wails build
```

## 默认连接
- URL: `ws://127.0.0.1:8099/ws`
- Agent: `main`

首次打开请在右上角【设置】中填写 token 并连接。
