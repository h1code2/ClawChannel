const els = {
  app: document.getElementById('app'),
  sidebarResizer: document.getElementById('sidebarResizer'),
  sessionList: document.getElementById('sessionList'),
  sessionSearch: document.getElementById('sessionSearch'),
  newSessionBtn: document.getElementById('newSessionBtn'),
  chatTitle: document.getElementById('chatTitle'),
  chatSub: document.getElementById('chatSub'),
  messageViewport: document.getElementById('messageViewport'),
  composerInput: document.getElementById('composerInput'),
  sendBtn: document.getElementById('sendBtn'),
  openSettingsBtn: document.getElementById('openSettingsBtn'),
  closeSettingsBtn: document.getElementById('closeSettingsBtn'),
  settingsPanel: document.getElementById('settingsPanel'),
  gatewayUrl: document.getElementById('gatewayUrl'),
  gatewayToken: document.getElementById('gatewayToken'),
  agentSelect: document.getElementById('agentSelect'),
  connectBtn: document.getElementById('connectBtn'),
  disconnectBtn: document.getElementById('disconnectBtn')
};

const STORAGE_KEY = 'clawchannel.wails.state.v1';

const state = {
  sessions: [{ id: crypto.randomUUID(), name: '默认会话', messages: [] }],
  selectedSessionId: null,
  ws: null,
  reconnectTimer: null,
  reconnectAttempt: 0,
  reconnectEnabled: true,
  ui: {
    sidebarWidth: 320,
    hasSavedConfig: false
  },
  config: {
    gatewayUrl: 'ws://127.0.0.1:8099/ws',
    token: '',
    agent: 'main'
  },
  connectionText: '未连接'
};

function loadState() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    const saved = JSON.parse(raw);
    if (saved?.config) state.config = { ...state.config, ...saved.config };
    if (saved?.ui) state.ui = { ...state.ui, ...saved.ui };
    if (Array.isArray(saved?.sessions) && saved.sessions.length > 0) {
      state.sessions = saved.sessions.map((s) => ({
        id: s.id || crypto.randomUUID(),
        name: s.name || '会话',
        messages: Array.isArray(s.messages) ? s.messages : []
      }));
    }
    state.selectedSessionId = saved?.selectedSessionId || state.sessions[0]?.id;

    // Backward compatibility: old state may have config but no hasSavedConfig flag.
    if (!saved?.ui || typeof saved.ui.hasSavedConfig === 'undefined') {
      if ((state.config.gatewayUrl || '').trim() !== '') {
        state.ui.hasSavedConfig = true;
      }
    }
  } catch (_) {}
}

function saveState() {
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({
      sessions: state.sessions,
      selectedSessionId: state.selectedSessionId,
      config: state.config,
      ui: state.ui
    })
  );
}

function nowTS() {
  return Date.now();
}

function fmtTime(ts) {
  const d = new Date(ts);
  return d.toLocaleTimeString('zh-CN', { hour12: false });
}

function currentSession() {
  return state.sessions.find((s) => s.id === state.selectedSessionId) || state.sessions[0];
}

function setConnectionText(text) {
  state.connectionText = text;
  const s = currentSession();
  els.chatSub.textContent = `${text} · Agent ${state.config.agent}`;
  if (s) {
    els.chatTitle.textContent = s.name;
  }
}

function applySidebarWidth() {
  const width = Math.max(240, Math.min(520, Number(state.ui.sidebarWidth) || 320));
  state.ui.sidebarWidth = width;
  document.documentElement.style.setProperty('--sidebar-width', `${width}px`);
}

function bindSidebarResize() {
  if (!els.sidebarResizer) return;

  let dragging = false;

  const onMove = (clientX) => {
    const appRect = els.app.getBoundingClientRect();
    const next = clientX - appRect.left;
    state.ui.sidebarWidth = Math.max(240, Math.min(520, next));
    applySidebarWidth();
  };

  const stop = () => {
    if (!dragging) return;
    dragging = false;
    els.sidebarResizer.classList.remove('dragging');
    document.body.style.userSelect = '';
    saveState();
    window.removeEventListener('mousemove', onMouseMove);
    window.removeEventListener('mouseup', onMouseUp);
    window.removeEventListener('touchmove', onTouchMove);
    window.removeEventListener('touchend', onTouchEnd);
  };

  const onMouseMove = (e) => {
    if (!dragging) return;
    onMove(e.clientX);
  };

  const onTouchMove = (e) => {
    if (!dragging || !e.touches?.length) return;
    onMove(e.touches[0].clientX);
  };

  const onMouseUp = () => stop();
  const onTouchEnd = () => stop();

  const start = (clientX) => {
    dragging = true;
    els.sidebarResizer.classList.add('dragging');
    document.body.style.userSelect = 'none';
    onMove(clientX);
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    window.addEventListener('touchmove', onTouchMove, { passive: true });
    window.addEventListener('touchend', onTouchEnd);
  };

  els.sidebarResizer.addEventListener('mousedown', (e) => {
    e.preventDefault();
    start(e.clientX);
  });

  els.sidebarResizer.addEventListener('touchstart', (e) => {
    if (!e.touches?.length) return;
    start(e.touches[0].clientX);
  }, { passive: true });
}

function renderSessionList() {
  const kw = (els.sessionSearch.value || '').trim().toLowerCase();
  els.sessionList.innerHTML = '';
  for (const s of state.sessions) {
    const last = s.messages[s.messages.length - 1];
    const preview = last?.text || '暂无消息';
    if (kw && !(s.name.toLowerCase().includes(kw) || preview.toLowerCase().includes(kw))) continue;

    const btn = document.createElement('button');
    btn.className = `session-item ${s.id === state.selectedSessionId ? 'active' : ''}`;
    btn.innerHTML = `
      <div class="session-name">${escapeHtml(s.name)}</div>
      <div class="session-preview">${escapeHtml(preview)}</div>
    `;
    btn.onclick = () => {
      state.selectedSessionId = s.id;
      renderAll();
      saveState();
    };
    els.sessionList.appendChild(btn);
  }
}

function renderMessages() {
  const s = currentSession();
  if (!s) return;
  els.chatTitle.textContent = s.name;
  els.chatSub.textContent = `${state.connectionText} · Agent ${state.config.agent}`;
  els.messageViewport.innerHTML = '';

  for (const m of s.messages) {
    const row = document.createElement('div');
    row.className = `msg-row ${m.from === 'me' ? 'me' : 'bot'}`;
    const bubble = document.createElement('div');
    bubble.className = 'msg-bubble';
    bubble.innerHTML = `
      <div class="msg-meta">${m.from === 'me' ? 'You' : 'Assistant'} · ${fmtTime(m.ts)}</div>
      <div class="msg-rich">${renderRichTextLimited(m.text)}</div>
    `;
    row.appendChild(bubble);
    els.messageViewport.appendChild(row);
  }

  const bottom = document.createElement('div');
  bottom.style.height = '14px';
  els.messageViewport.appendChild(bottom);
  els.messageViewport.scrollTop = els.messageViewport.scrollHeight;
}

function renderAll() {
  renderSessionList();
  renderMessages();
}

function addMessage(from, text, eventType = 'user_message') {
  const s = currentSession();
  if (!s) return;
  s.messages.push({ from, text: normalizeAssistantText(text), eventType, ts: nowTS() });
  if (s.messages.length > 1200) s.messages = s.messages.slice(-1200);
  renderAll();
  saveState();
}

function normalizeAssistantText(text) {
  const t = (text || '').trim();
  if (!t) return '';
  try {
    const obj = JSON.parse(t);
    const ext = extractAnyText(obj);
    return ext || t;
  } catch {
    return t;
  }
}

function extractAnyText(v) {
  if (typeof v === 'string') return v.trim();
  if (Array.isArray(v)) {
    for (let i = v.length - 1; i >= 0; i--) {
      const x = extractAnyText(v[i]);
      if (x) return x;
    }
    return '';
  }
  if (v && typeof v === 'object') {
    for (const k of ['reply', 'output_text', 'text', 'message', 'output', 'result', 'response', 'final']) {
      if (k in v) {
        const x = extractAnyText(v[k]);
        if (x) return x;
      }
    }
    for (const val of Object.values(v)) {
      const x = extractAnyText(val);
      if (x) return x;
    }
  }
  return '';
}

function makeEnv(type, payload, needAck = false) {
  return {
    v: 1,
    type,
    msgId: `c_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
    ts: Date.now(),
    needAck,
    payload
  };
}

function sendEnvelope(type, payload, needAck = false) {
  if (!state.ws || state.ws.readyState !== WebSocket.OPEN) throw new Error('未连接网关');
  state.ws.send(JSON.stringify(makeEnv(type, payload, needAck)));
}

function connectWS() {
  disconnectWS(false);

  let url = (state.config.gatewayUrl || '').trim();
  if (!url) throw new Error('Gateway URL 不能为空');

  const u = new URL(url);
  if (state.config.token) u.searchParams.set('token', state.config.token);

  const ws = new WebSocket(u.toString());
  state.ws = ws;
  setConnectionText('连接中...');

  ws.onopen = () => {
    state.reconnectAttempt = 0;
    setConnectionText('已连接');
    sendEnvelope('hello', { client: 'wails-webview' }, false);
    if (state.config.token) {
      sendEnvelope('auth', { token: state.config.token }, false);
    }
    sendEnvelope('agent.select', { agentId: state.config.agent }, false);
  };

  ws.onmessage = (evt) => {
    let env;
    try {
      env = JSON.parse(evt.data);
    } catch {
      return;
    }

    if (env?.needAck && env?.msgId) {
      try {
        sendEnvelope('ack', { ackMsgId: env.msgId, status: 'received' }, false);
      } catch (_) {}
    }

    if (env?.type === 'ack') return;

    if (env?.type === 'error') {
      const msg = env?.payload?.message || env?.payload?.code || '未知错误';
      addMessage('assistant', `⚠️ ${msg}`, 'error');
      return;
    }

    if (env?.type === 'command') {
      const cmd = env?.payload?.command;
      if (cmd === 'auth.ok') setConnectionText('已鉴权');
      if (cmd === 'agent.selected') setConnectionText('已连接');
      return;
    }

    if (env?.type === 'event') {
      const p = env.payload || {};
      if (p.eventType === 'assistant_stream' && p.delta) {
        const s = currentSession();
        const last = s.messages[s.messages.length - 1];
        if (last && last.from === 'assistant' && last.eventType === 'assistant_stream') {
          last.text += p.delta;
          last.ts = nowTS();
          renderMessages();
          saveState();
          return;
        }
        addMessage('assistant', p.delta, 'assistant_stream');
        return;
      }
      if (p.eventType === 'assistant_message') {
        addMessage('assistant', p.text || '', 'assistant_message');
      }
    }
  };

  ws.onclose = () => {
    setConnectionText('连接已断开');
    state.ws = null;
    if (!state.reconnectEnabled) return;
    const backoff = Math.min(12000, 1000 * Math.pow(1.7, state.reconnectAttempt++));
    state.reconnectTimer = setTimeout(() => {
      try {
        connectWS();
      } catch (_) {}
    }, backoff);
  };

  ws.onerror = () => {
    setConnectionText('连接错误');
  };
}

function disconnectWS(manual = true) {
  if (manual) state.reconnectEnabled = false;
  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
  }
  if (state.ws) {
    try { state.ws.close(); } catch (_) {}
    state.ws = null;
  }
  setConnectionText('未连接');
}

function sendText() {
  const text = (els.composerInput.value || '').trim();
  if (!text) return;
  addMessage('me', text, 'user_message');
  try {
    sendEnvelope('event', { eventType: 'user_message', text, agentId: state.config.agent }, true);
  } catch (err) {
    addMessage('assistant', `⚠️ ${err.message || err}`);
  }
  els.composerInput.value = '';
  autoGrowInput();
}

function autoGrowInput() {
  const el = els.composerInput;
  el.style.height = 'auto';
  el.style.height = `${Math.min(el.scrollHeight, 220)}px`;
}

function createSession() {
  const id = crypto.randomUUID();
  state.sessions.unshift({ id, name: `会话 ${state.sessions.length + 1}`, messages: [] });
  state.selectedSessionId = id;
  renderAll();
  saveState();
}

function openSettings(open) {
  els.settingsPanel.classList.toggle('hidden', !open);
}

function copyText(text) {
  const content = String(text || '');
  if (!content) return Promise.resolve(false);

  if (navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(content).then(() => true).catch(() => false);
  }

  try {
    const ta = document.createElement('textarea');
    ta.value = content;
    ta.setAttribute('readonly', 'readonly');
    ta.style.position = 'fixed';
    ta.style.left = '-9999px';
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return Promise.resolve(!!ok);
  } catch (_) {
    return Promise.resolve(false);
  }
}

function bindCodeCopyButtons() {
  els.messageViewport.addEventListener('click', async (e) => {
    const btn = e.target.closest('.rt-copy-btn');
    if (!btn) return;

    const block = btn.closest('.rt-code-block');
    const codeEl = block?.querySelector('code');
    if (!codeEl) return;

    const ok = await copyText(codeEl.textContent || '');
    const origin = btn.textContent;
    btn.textContent = ok ? '已复制' : '复制失败';
    btn.classList.toggle('ok', ok);
    btn.classList.toggle('err', !ok);

    setTimeout(() => {
      btn.textContent = origin || '复制';
      btn.classList.remove('ok', 'err');
    }, 1200);
  });
}

function bindEvents() {
  els.newSessionBtn.onclick = createSession;
  els.sessionSearch.oninput = renderSessionList;
  els.sendBtn.onclick = sendText;
  els.composerInput.oninput = autoGrowInput;
  els.composerInput.onkeydown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendText();
    }
  };

  els.openSettingsBtn.onclick = () => openSettings(true);
  els.closeSettingsBtn.onclick = () => openSettings(false);

  els.connectBtn.onclick = () => {
    state.reconnectEnabled = true;
    state.config.gatewayUrl = els.gatewayUrl.value.trim();
    state.config.token = els.gatewayToken.value.trim();
    state.config.agent = els.agentSelect.value;
    state.ui.hasSavedConfig = true;
    saveState();
    connectWS();
    openSettings(false);
  };

  els.disconnectBtn.onclick = () => disconnectWS(true);

  window.addEventListener('beforeunload', () => {
    saveState();
    disconnectWS(true);
  });
}

function hydrateConfigUI() {
  els.gatewayUrl.value = state.config.gatewayUrl;
  els.gatewayToken.value = state.config.token;
  els.agentSelect.value = state.config.agent;
}

function escapeHtml(str) {
  return String(str)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

function sanitizeUrl(raw) {
  const s = String(raw || '').trim();
  if (!s) return null;
  try {
    const u = new URL(s.startsWith('www.') ? `https://${s}` : s);
    if (u.protocol === 'http:' || u.protocol === 'https:') return u.toString();
  } catch (_) {}
  return null;
}

function renderInlineRich(text) {
  let s = escapeHtml(text);

  // links: auto-link plain urls
  s = s.replace(/(https?:\/\/[^\s<]+[^<.,:;"')\]\s])/g, (m) => {
    const safe = sanitizeUrl(m);
    if (!safe) return m;
    return `<a href="${safe}" target="_blank" rel="noopener noreferrer">${m}</a>`;
  });

  // inline code
  s = s.replace(/`([^`\n]+)`/g, '<code>$1</code>');
  // bold / italic / strike (limited markdown)
  s = s.replace(/\*\*([^*\n][\s\S]*?[^*\n])\*\*/g, '<strong>$1</strong>');
  s = s.replace(/(^|[^*])\*([^*\n][\s\S]*?[^*\n])\*(?!\*)/g, '$1<em>$2</em>');
  s = s.replace(/~~([^~\n][\s\S]*?[^~\n])~~/g, '<del>$1</del>');

  return s;
}

function renderRichTextLimited(text) {
  const src = String(text || '');
  const lines = src.split('\n');
  const out = [];
  let inCode = false;
  let codeLang = '';
  let codeBuf = [];

  const flushCode = () => {
    if (!codeBuf.length) return;
    const body = escapeHtml(codeBuf.join('\n'));
    const lang = escapeHtml(codeLang || 'text');
    out.push(
      `<div class="rt-code-block">` +
        `<div class="rt-code-head">` +
          `<span class="msg-meta">${lang}</span>` +
          `<button class="rt-copy-btn" type="button">复制</button>` +
        `</div>` +
        `<code>${body}</code>` +
      `</div>`
    );
    codeBuf = [];
  };

  for (const line of lines) {
    const fence = line.match(/^```\s*([^`]*)$/);
    if (fence) {
      if (!inCode) {
        inCode = true;
        codeLang = (fence[1] || '').trim();
      } else {
        inCode = false;
        flushCode();
        codeLang = '';
      }
      continue;
    }

    if (inCode) {
      codeBuf.push(line);
      continue;
    }

    out.push(renderInlineRich(line));
  }

  if (inCode) {
    flushCode();
  }

  return out.join('<br/>');
}

(function bootstrap() {
  loadState();
  if (!state.selectedSessionId && state.sessions.length) {
    state.selectedSessionId = state.sessions[0].id;
  }
  applySidebarWidth();
  hydrateConfigUI();
  bindEvents();
  bindCodeCopyButtons();
  bindSidebarResize();
  renderAll();
  autoGrowInput();
  setConnectionText('未连接');

  if (state.ui.hasSavedConfig) {
    state.reconnectEnabled = true;
    setTimeout(() => {
      try {
        connectWS();
      } catch (err) {
        setConnectionText(`自动连接失败: ${err?.message || err}`);
      }
    }, 180);
  }
})();
