# Channel Gateway (MVP+)

最小可运行网关，职责：
- 接入客户端 WebSocket
- token/JWT 鉴权
- 协议解析（hello/auth/ping/event/ack/agent.select）
- 根据 agent 路由到 OpenClaw（`openclaw agent --json`）
- 回复回推给 WS client（支持可选 streaming）
- ACK + 投递状态
- 全链路日志
- 落盘消息（JSONL）
- 基础指标（/metrics）
- 最小 OAuth token endpoint

## 运行

```bash
cd services/channel-gateway
go mod tidy
go run .
```

## 环境变量
- `LISTEN_ADDR`：监听地址，默认 `:8099`
- `DEFAULT_AGENT`：默认 agent，默认 `main`
- `OPENCLAW_BIN`：默认 `openclaw`
- `DATA_DIR`：消息落盘目录，默认 `./data`
- `ENABLE_STREAMING`：`true/false`，默认 `false`

### 鉴权
- `AUTH_MODE=token`（默认）
  - 需要：`GATEWAY_TOKEN`
- `AUTH_MODE=jwt`
  - 需要：`JWT_SECRET`
  - 客户端通过 query `jwt=...` 或 auth payload 提交 JWT

### OAuth（可选）
- `OAUTH_CLIENT_ID`
- `OAUTH_CLIENT_SECRET`
- `JWT_SECRET`

启用后可调用：
- `POST /oauth/token`（client_id/client_secret）换取 JWT

## 协议 Envelope

```json
{
  "v":1,
  "type":"hello|auth|event|ack|error|agent.select|command",
  "msgId":"uuid",
  "ts":1700000000000,
  "needAck":true,
  "payload":{}
}
```

## 指标
- `GET /metrics`
  - requests
  - routeOk / routeFailed / routeRetries
  - routeP95Ms
  - routeRetryRatio
  - connectedSessions
