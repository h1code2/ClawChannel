# ClawChannel (mac) — 最小可运行两服务功能清单（Fyne 版）

> 目标：mac 自定义 Channel 与 OpenClaw 对接，可稳定收发消息、多 agent 路由、基础可用。

## 服务 1：Go Fyne Client（桌面客户端）

### 1.1 必须功能
- **登录/配置**
  - 配置 Gateway WebSocket 地址（wss://...）
  - 配置 Token（或从登录流程获取）
  - 保存到本地配置（Preferences）

- **会话与消息 UI**
  - 会话列表（可简单，本地维护）
  - 聊天界面（发送/展示）
  - 基础消息气泡/分区（自己/对方区分，最小可用）

- **连接层**
  - WebSocket 长连接
  - 断线重连（指数退避）
  - 心跳（ping/pong）
  - ACK 机制（消息投递确认）

- **发送消息**
  - 发送文本消息
  - 生成 msgId / ts / needAck

- **接收消息**
  - 处理 `event` / `command` / `ack` / `error`
  - 展示 assistant 回复

- **Agent 选择（最小）**
  - UI 选择 agent（main/developer/daily）
  - 发送 `agent.select` 或在消息中附带 `agentId`

### 1.2 可选（非必须）
- 消息搜索 / 多会话持久化
- 通知中心推送
- 语音/图片

---

## 服务 2：Channel Gateway（中转 + 路由）

### 2.1 必须功能
- **WebSocket 接入**
  - 接受客户端连接
  - 校验 token（最小可用）
  - 维护 sessionId（可内存/简单存储）

- **协议解析/校验**
  - Envelope: v/type/msgId/ts/needAck/payload
  - 解析 hello / auth / ping / event / ack / agent.select

- **路由到 OpenClaw**
  - 将 `event` 转换为 OpenClaw 消息
  - 根据 agentId 路由到目标 agent session
  - 支持默认 agent fallback

- **消息回推**
  - OpenClaw 回复 -> Gateway -> WS client
  - 支持 streaming（可选）

- **ACK 机制**
  - 收到消息后回 ack
  - 网关内部记录投递状态（最小可内存）

- **基础日志**
  - request_id / session_id / msg_id 全链路日志
  - error 日志（可先打印）

### 2.2 可选（非必须）
- 认证服务（OAuth / JWT）
- 存储服务（会话+消息落盘）
- 监控指标（P95 延迟 / 重试率）

---

## 建议的最小协议（摘要）

### Envelope
```json
{
  "v": 1,
  "type": "hello|auth|event|ack|error|agent.select",
  "msgId": "uuid",
  "ts": 1700000000000,
  "needAck": true,
  "payload": {}
}
```

### event payload
```json
{
  "eventType": "user_message",
  "text": "...",
  "agentId": "main"
}
```

---

## 可运行 MVP 判定
以下全部满足即可对外演示：
- Fyne Client 能连上网关
- 能发送消息，网关能路由到 OpenClaw
- 能收到 OpenClaw 回复并显示
- 能切换 agent 并生效
