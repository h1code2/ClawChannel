package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

type Envelope struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	MsgID   string          `json:"msgId"`
	TS      int64           `json:"ts"`
	NeedAck bool            `json:"needAck"`
	Payload json.RawMessage `json:"payload"`
}

type EventPayload struct {
	EventType string         `json:"eventType"`
	Text      string         `json:"text"`
	AgentID   string         `json:"agentId,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

type AuthPayload struct {
	Token string `json:"token"`
	JWT   string `json:"jwt,omitempty"`
}

type AgentSelectPayload struct {
	AgentID string `json:"agentId"`
}

type AckPayload struct {
	AckMsgID string `json:"ackMsgId"`
	Status   string `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type Session struct {
	id            string
	conn          *websocket.Conn
	authed        bool
	selectedAgent string
	remoteAddr    string
	userID        string
	mu            sync.Mutex
}

type PersistRecord struct {
	TS        time.Time      `json:"ts"`
	Type      string         `json:"type"`
	SessionID string         `json:"sessionId"`
	MsgID     string         `json:"msgId,omitempty"`
	AgentID   string         `json:"agentId,omitempty"`
	Role      string         `json:"role,omitempty"`
	Text      string         `json:"text,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type Metrics struct {
	mu              sync.Mutex
	Requests        int64     `json:"requests"`
	RouteOK         int64     `json:"routeOk"`
	RouteFailed     int64     `json:"routeFailed"`
	RouteRetries    int64     `json:"routeRetries"`
	AckSent         int64     `json:"ackSent"`
	Connected       int64     `json:"connectedSessions"`
	StartedAt       time.Time `json:"startedAt"`
	RouteLatencyMS  []float64 `json:"-"`
	RouteP95MS      float64   `json:"routeP95Ms"`
	RouteRetryRatio float64   `json:"routeRetryRatio"`
}

type Server struct {
	token        string
	defaultAgent string
	listenAddr   string
	openclawBin  string
	channelName  string
	authMode     string // token|jwt
	jwtSecret    string
	dataDir      string
	enableStream bool

	oauthClientID     string
	oauthClientSecret string

	sessionsMu sync.RWMutex
	sessions   map[string]*Session

	deliveryMu sync.RWMutex
	delivery   map[string]string

	metrics Metrics
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func main() {
	s := &Server{
		token:             strings.TrimSpace(os.Getenv("GATEWAY_TOKEN")),
		defaultAgent:      envOr("DEFAULT_AGENT", "main"),
		listenAddr:        envOr("LISTEN_ADDR", ":8099"),
		openclawBin:       envOr("OPENCLAW_BIN", "openclaw"),
		channelName:       envOr("CHANNEL_NAME", "telegram"),
		authMode:          strings.ToLower(envOr("AUTH_MODE", "token")),
		jwtSecret:         strings.TrimSpace(os.Getenv("JWT_SECRET")),
		dataDir:           envOr("DATA_DIR", "./data"),
		enableStream:      strings.EqualFold(envOr("ENABLE_STREAMING", "false"), "true"),
		oauthClientID:     strings.TrimSpace(os.Getenv("OAUTH_CLIENT_ID")),
		oauthClientSecret: strings.TrimSpace(os.Getenv("OAUTH_CLIENT_SECRET")),
		sessions:          make(map[string]*Session),
		delivery:          make(map[string]string),
		metrics:           Metrics{StartedAt: time.Now()},
	}

	if s.authMode == "token" && s.token == "" {
		log.Fatal("missing required env: GATEWAY_TOKEN when AUTH_MODE=token")
	}
	if s.authMode == "jwt" && s.jwtSecret == "" {
		log.Fatal("missing required env: JWT_SECRET when AUTH_MODE=jwt")
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		log.Fatalf("mkdir data dir: %v", err)
	}

	http.HandleFunc("/ws", s.handleWS)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	http.HandleFunc("/metrics", s.handleMetrics)
	http.HandleFunc("/oauth/token", s.handleOAuthToken)

	log.Printf("channel-gateway listening on %s auth_mode=%s", s.listenAddr, s.authMode)
	if err := http.ListenAndServe(s.listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if s.oauthClientID == "" || s.oauthClientSecret == "" || s.jwtSecret == "" {
		http.Error(w, "oauth disabled", http.StatusNotImplemented)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	cid := strings.TrimSpace(r.Form.Get("client_id"))
	sec := strings.TrimSpace(r.Form.Get("client_secret"))
	if cid != s.oauthClientID || sec != s.oauthClientSecret {
		http.Error(w, "invalid client", http.StatusUnauthorized)
		return
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": cid,
		"iat": now.Unix(),
		"exp": now.Add(12 * time.Hour).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok, err := t.SignedString([]byte(s.jwtSecret))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"access_token": tok, "token_type": "bearer", "expires_in": 43200})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	m := s.metrics
	m.RouteP95MS = p95(m.RouteLatencyMS)
	if m.Requests > 0 {
		m.RouteRetryRatio = float64(m.RouteRetries) / float64(m.Requests)
	}
	writeJSON(w, m)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	sessionID := fmt.Sprintf("sess_%d", time.Now().UnixNano())
	sess := &Session{id: sessionID, conn: conn, selectedAgent: s.defaultAgent, remoteAddr: r.RemoteAddr}

	if s.authMode == "token" {
		if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" && token == s.token {
			sess.authed = true
		}
	} else if s.authMode == "jwt" {
		if raw := strings.TrimSpace(r.URL.Query().Get("jwt")); raw != "" {
			if sub, ok := s.verifyJWT(raw); ok {
				sess.authed, sess.userID = true, sub
			}
		}
	}

	s.sessionsMu.Lock()
	s.sessions[sessionID] = sess
	s.metrics.mu.Lock()
	s.metrics.Connected = int64(len(s.sessions))
	s.metrics.mu.Unlock()
	s.sessionsMu.Unlock()
	s.persist(PersistRecord{TS: time.Now(), Type: "session_connected", SessionID: sessionID, Meta: map[string]any{"remote": sess.remoteAddr}})
	log.Printf("session connected session_id=%s remote=%s authed=%t", sessionID, sess.remoteAddr, sess.authed)

	defer func() {
		s.sessionsMu.Lock()
		delete(s.sessions, sessionID)
		s.metrics.mu.Lock()
		s.metrics.Connected = int64(len(s.sessions))
		s.metrics.mu.Unlock()
		s.sessionsMu.Unlock()
		s.persist(PersistRecord{TS: time.Now(), Type: "session_closed", SessionID: sessionID})
		_ = conn.Close()
	}()

	conn.SetReadLimit(2 << 20)
	conn.SetPongHandler(func(_ string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				return
			}
			log.Printf("read error session_id=%s err=%v", sessionID, err)
			return
		}
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			s.pushError(sess, "bad_envelope", "invalid JSON envelope", "")
			continue
		}
		if env.V == 0 {
			env.V = 1
		}
		if env.NeedAck {
			s.pushAck(sess, env.MsgID, "received", "")
		}

		reqID := fmt.Sprintf("req_%d", time.Now().UnixNano())
		s.metrics.mu.Lock()
		s.metrics.Requests++
		s.metrics.mu.Unlock()
		log.Printf("request request_id=%s session_id=%s msg_id=%s type=%s", reqID, sessionID, env.MsgID, env.Type)

		if !sess.authed {
			if env.Type != "auth" && env.Type != "hello" {
				s.pushError(sess, "unauthorized", "auth required", env.MsgID)
				continue
			}
		}
		s.handleEnvelope(reqID, sess, env)
	}
}

func (s *Server) verifyJWT(raw string) (string, bool) {
	tok, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil || !tok.Valid {
		return "", false
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		sub = "jwt-user"
	}
	return sub, true
}

func (s *Server) handleEnvelope(requestID string, sess *Session, env Envelope) {
	switch env.Type {
	case "hello":
		s.pushCommand(sess, "hello.ok", map[string]any{"sessionId": sess.id, "agentId": sess.selectedAgent})
	case "auth":
		var p AuthPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			s.pushError(sess, "bad_auth", "invalid auth payload", env.MsgID)
			return
		}
		if s.authMode == "token" {
			if strings.TrimSpace(p.Token) != s.token {
				s.pushError(sess, "unauthorized", "invalid token", env.MsgID)
				return
			}
			sess.authed = true
		} else {
			raw := strings.TrimSpace(p.JWT)
			if raw == "" {
				raw = strings.TrimSpace(p.Token)
			}
			sub, ok := s.verifyJWT(raw)
			if !ok {
				s.pushError(sess, "unauthorized", "invalid jwt", env.MsgID)
				return
			}
			sess.authed, sess.userID = true, sub
		}
		s.pushCommand(sess, "auth.ok", map[string]any{"sessionId": sess.id})
	case "ping":
		s.pushCommand(sess, "pong", map[string]any{"ts": time.Now().UnixMilli()})
	case "ack":
		var p AckPayload
		_ = json.Unmarshal(env.Payload, &p)
		s.markDelivery(p.AckMsgID, "client_acked")
	case "agent.select":
		var p AgentSelectPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || strings.TrimSpace(p.AgentID) == "" {
			s.pushError(sess, "bad_agent", "agentId required", env.MsgID)
			return
		}
		sess.mu.Lock()
		sess.selectedAgent = strings.TrimSpace(p.AgentID)
		sess.mu.Unlock()
		s.pushCommand(sess, "agent.selected", map[string]any{"agentId": p.AgentID})
	case "event":
		var p EventPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			s.pushError(sess, "bad_event", "invalid event payload", env.MsgID)
			return
		}
		if strings.TrimSpace(p.Text) == "" {
			s.pushError(sess, "bad_event", "text required", env.MsgID)
			return
		}
		agent := strings.TrimSpace(p.AgentID)
		if agent == "" {
			sess.mu.Lock()
			agent = sess.selectedAgent
			sess.mu.Unlock()
		}
		if agent == "" {
			agent = s.defaultAgent
		}
		s.persist(PersistRecord{TS: time.Now(), Type: "message", SessionID: sess.id, MsgID: env.MsgID, AgentID: agent, Role: "user", Text: p.Text, Meta: map[string]any{"eventType": p.EventType}})
		s.processUserMessage(requestID, sess, env.MsgID, agent, p.Text)
	default:
		s.pushError(sess, "unsupported_type", "unsupported envelope type", env.MsgID)
	}
}

func (s *Server) processUserMessage(requestID string, sess *Session, msgID, agentID, text string) {
	s.markDelivery(msgID, "routing")
	start := time.Now()

	replyText, err := s.routeWithRetry(agentID, text)
	lat := time.Since(start)
	s.metrics.mu.Lock()
	s.metrics.RouteLatencyMS = append(s.metrics.RouteLatencyMS, float64(lat.Milliseconds()))
	if len(s.metrics.RouteLatencyMS) > 500 {
		s.metrics.RouteLatencyMS = s.metrics.RouteLatencyMS[len(s.metrics.RouteLatencyMS)-500:]
	}
	s.metrics.mu.Unlock()

	if err != nil {
		s.markDelivery(msgID, "failed")
		s.metrics.mu.Lock()
		s.metrics.RouteFailed++
		s.metrics.mu.Unlock()
		log.Printf("route failed request_id=%s session_id=%s msg_id=%s agent=%s err=%v", requestID, sess.id, msgID, agentID, err)
		s.pushError(sess, "route_failed", err.Error(), msgID)
		return
	}

	s.metrics.mu.Lock()
	s.metrics.RouteOK++
	s.metrics.mu.Unlock()
	s.markDelivery(msgID, "delivered")
	log.Printf("route ok request_id=%s session_id=%s msg_id=%s agent=%s", requestID, sess.id, msgID, agentID)

	if s.enableStream {
		for _, chunk := range splitChunks(replyText, 120) {
			s.pushEnvelope(sess, Envelope{V: 1, Type: "event", MsgID: fmt.Sprintf("srv_%d", time.Now().UnixNano()), TS: time.Now().UnixMilli(), Payload: mustJSON(map[string]any{
				"eventType": "assistant_stream",
				"delta":     chunk,
				"agentId":   agentID,
				"sessionId": sess.id,
			})})
		}
	}

	s.persist(PersistRecord{TS: time.Now(), Type: "message", SessionID: sess.id, MsgID: msgID, AgentID: agentID, Role: "assistant", Text: replyText})
	s.pushEnvelope(sess, Envelope{V: 1, Type: "event", MsgID: fmt.Sprintf("srv_%d", time.Now().UnixNano()), TS: time.Now().UnixMilli(), Payload: mustJSON(map[string]any{
		"eventType": "assistant_message",
		"text":      replyText,
		"agentId":   agentID,
		"sessionId": sess.id,
	})})
}

func (s *Server) routeWithRetry(agentID, text string) (string, error) {
	tries := 2
	var lastErr error
	for i := 0; i < tries; i++ {
		reply, err := s.callOpenClaw(agentID, text)
		if err == nil {
			return reply, nil
		}
		lastErr = err
		if i+1 < tries {
			s.metrics.mu.Lock()
			s.metrics.RouteRetries++
			s.metrics.mu.Unlock()
			time.Sleep(time.Duration(i+1) * 400 * time.Millisecond)
		}
	}
	return "", lastErr
}

func (s *Server) callOpenClaw(agentID, text string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	args := []string{"agent", "--agent", agentID, "--message", text, "--json"}
	cmd := exec.CommandContext(ctx, s.openclawBin, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf(detail)
	}
	reply := extractReplyText(out.Bytes())
	if reply == "" {
		reply = strings.TrimSpace(out.String())
	}
	return reply, nil
}

func extractReplyText(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	// openclaw --json may emit multiple JSON lines/events; prefer the last usable assistant text.
	lines := bytes.Split(trimmed, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var obj any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		if txt := extractTextFromAny(obj); txt != "" {
			return txt
		}
	}

	var obj any
	if err := json.Unmarshal(trimmed, &obj); err == nil {
		if txt := extractTextFromAny(obj); txt != "" {
			return txt
		}
	}
	return ""
}

func extractTextFromAny(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		for _, key := range []string{"reply", "output_text", "text", "message", "output", "result", "response", "final"} {
			if val, ok := t[key]; ok {
				if txt := extractTextFromAny(val); txt != "" {
					return txt
				}
			}
		}
		if msgs, ok := t["messages"]; ok {
			if txt := extractTextFromAny(msgs); txt != "" {
				return txt
			}
		}
		if content, ok := t["content"]; ok {
			if txt := extractTextFromAny(content); txt != "" {
				return txt
			}
		}
		for _, val := range t {
			if txt := extractTextFromAny(val); txt != "" {
				return txt
			}
		}
	case []any:
		for i := len(t) - 1; i >= 0; i-- {
			if txt := extractTextFromAny(t[i]); txt != "" {
				return txt
			}
		}
	}
	return ""
}

func (s *Server) pushAck(sess *Session, ackMsgID, status, detail string) {
	s.metrics.mu.Lock()
	s.metrics.AckSent++
	s.metrics.mu.Unlock()
	s.pushEnvelope(sess, Envelope{V: 1, Type: "ack", MsgID: fmt.Sprintf("ack_%d", time.Now().UnixNano()), TS: time.Now().UnixMilli(), Payload: mustJSON(AckPayload{AckMsgID: ackMsgID, Status: status, Detail: detail})})
}

func (s *Server) pushError(sess *Session, code, message, forMsgID string) {
	s.pushEnvelope(sess, Envelope{V: 1, Type: "error", MsgID: fmt.Sprintf("err_%d", time.Now().UnixNano()), TS: time.Now().UnixMilli(), Payload: mustJSON(map[string]any{"code": code, "message": message, "forMsgId": forMsgID})})
}

func (s *Server) pushCommand(sess *Session, command string, data map[string]any) {
	s.pushEnvelope(sess, Envelope{V: 1, Type: "command", MsgID: fmt.Sprintf("cmd_%d", time.Now().UnixNano()), TS: time.Now().UnixMilli(), Payload: mustJSON(map[string]any{"command": command, "data": data})})
}

func (s *Server) pushEnvelope(sess *Session, env Envelope) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.conn == nil {
		return
	}
	_ = sess.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := sess.conn.WriteJSON(env); err != nil {
		log.Printf("write error session_id=%s msg_id=%s type=%s err=%v", sess.id, env.MsgID, env.Type, err)
	}
}

func (s *Server) markDelivery(msgID, status string) {
	if strings.TrimSpace(msgID) == "" {
		return
	}
	s.deliveryMu.Lock()
	s.delivery[msgID] = status
	s.deliveryMu.Unlock()
}

func (s *Server) persist(r PersistRecord) {
	path := filepath.Join(s.dataDir, "events.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("persist open err: %v", err)
		return
	}
	defer f.Close()
	b, _ := json.Marshal(r)
	_, _ = f.Write(append(b, '\n'))
}

func splitChunks(s string, size int) []string {
	s = strings.TrimSpace(s)
	if s == "" || size <= 0 {
		return nil
	}
	runes := []rune(s)
	out := make([]string, 0, int(math.Ceil(float64(len(runes))/float64(size))))
	for i := 0; i < len(runes); i += size {
		j := i + size
		if j > len(runes) {
			j = len(runes)
		}
		out = append(out, string(runes[i:j]))
	}
	return out
}

func p95(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	idx := int(math.Ceil(float64(len(cp))*0.95)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}
