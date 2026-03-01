# ClawChannel MAC Minimal Services — 需求状态追踪（Wails版）

来源需求：`ClawChannel_MAC/ClawChannel_MAC_Minimal_Services.md`

> 规则：完成一项就记录一项。该文件按当前进度更新。

## 变更说明
- [x] 已删除 SwiftUI 客户端相关代码（`services/mac-app`）
- [x] 已删除 Fyne 客户端代码（`services/client-fyne`）
- [x] 已切换到 Wails 客户端路线（`services/client-wails`）

---

## 服务 1：Wails Client（桌面客户端）

### 1.1 必须功能
- [x] 登录/配置
  - [x] Gateway WebSocket 地址配置
  - [x] Token 配置
  - [x] Agent 选择（main/developer/daily）
  - [x] 本地保存配置

- [x] 会话与消息 UI
  - [x] 左侧会话列表
  - [x] 右侧聊天区（Telegram Desktop 风格深色布局）
  - [x] 自己/对方消息气泡区分
  - [x] 输入区右侧发送按钮

- [x] 连接层
  - [x] WebSocket 长连接
  - [x] 断线自动重连（指数退避）
  - [x] ACK 机制（收到 needAck 后回 ack）

- [x] 发送消息
  - [x] 文本发送
  - [x] Enter 发送 / Shift+Enter 换行
  - [x] `msgId / ts / needAck` 生成

- [x] 接收消息
  - [x] 处理 `event`
  - [x] 处理 `command`
  - [x] 处理 `ack`
  - [x] 处理 `error`
  - [x] 展示 assistant 回复
  - [x] assistant stream 分片拼接

### 1.2 可选（非必须）
- [x] 会话本地持久化（localStorage）
- [x] 会话搜索（左侧）
- [ ] 主题切换（浅色/深色）
- [ ] 更细腻动画（消息出现/面板过渡）

---

## 服务 2：Channel Gateway（中转 + 路由）

### 2.1 必须功能
- [x] WebSocket 接入（`/ws`）
- [x] Token/JWT 鉴权
- [x] Envelope 协议解析（hello/auth/ping/event/ack/agent.select）
- [x] 路由到 OpenClaw
- [x] 消息回推（含可选 streaming）
- [x] ACK 机制
- [x] 链路日志

### 2.2 可选（非必须）
- [x] OAuth/JWT 最小支持
- [x] 会话+消息落盘 JSONL
- [x] 指标监控 `/metrics`

---

## MVP 判定
- [x] 客户端可连上网关
- [x] 可发送并路由到 OpenClaw
- [x] 可收到并展示 OpenClaw 回复
- [x] Agent 切换可生效

---

## 当前验证状态
- [x] Gateway: `go build` 在当前环境通过
- [ ] Wails 客户端：本环境未安装 `wails` CLI，待 Mac 本机执行 `wails dev` 验证
