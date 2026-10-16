// ASF-Go Android 客户端交互逻辑
// 通过 asf-go IPC API 与服务器通信：GET /api/asf 状态、POST /api/command 命令

function switchPage(name) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.getElementById('page-' + name).classList.add('active');
  document.querySelectorAll('.nav-item').forEach(n => {
    n.classList.toggle('active', n.dataset.page === name);
  });
}

function toggleBotDetail(name) {
  const el = document.getElementById('detail-' + name);
  el.classList.toggle('hidden');
}

// ---- 服务器连接 ----
function getBase() {
  const host = document.getElementById('server-addr').value || '127.0.0.1';
  const port = document.getElementById('server-port').value || '1242';
  return 'http://' + host + ':' + port;
}

function getPass() {
  const el = document.querySelector('#page-settings input[type=password]');
  return el ? el.value : '';
}

function setConn(online, label) {
  const pill = document.getElementById('conn-pill');
  const labelEl = document.getElementById('conn-label');
  if (online) {
    pill.classList.remove('offline');
    labelEl.textContent = label || '已连接';
  } else {
    pill.classList.add('offline');
    labelEl.textContent = label || '离线';
  }
}

async function updateConn() {
  const host = document.getElementById('server-addr').value || '127.0.0.1';
  const port = document.getElementById('server-port').value || '1242';
  const base = getBase();
  const pass = getPass();
  try {
    const resp = await fetch(base + '/api/asf', {
      headers: { 'Authentication': pass }
    });
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    const data = await resp.json();
    const ver = data.Result && data.Result.Version ? data.Result.Version : '?';
    setConn(true, '已连接 v' + ver);
    appendCmd('> 连接 ' + host + ':' + port + ' … 握手成功 (v' + ver + ')', 'ok');
    refreshBots();
    return true;
  } catch (e) {
    setConn(false, '离线');
    appendCmd('> 连接 ' + host + ':' + port + ' … 失败: ' + e.message, 'err');
    return false;
  }
}

// ---- 命令执行 ----
async function runCommand(cmd) {
  const base = getBase();
  const pass = getPass();
  try {
    const resp = await fetch(base + '/api/command', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authentication': pass },
      body: JSON.stringify({ command: cmd })
    });
    const data = await resp.json();
    const msg = data.Message || data.Result || (data.Success ? 'OK' : '命令失败');
    appendCmd(String(msg), data.Success ? 'ok' : 'err');
  } catch (e) {
    appendCmd('请求失败: ' + e.message, 'err');
  }
}

function sendCommand() {
  const input = document.getElementById('cmd-input');
  const v = input.value.trim();
  if (!v) return;
  appendCmd('!' + v);
  input.value = '';
  runCommand(v);
}

function sendQuick(cmd) {
  appendCmd('!' + cmd);
  runCommand(cmd);
}

// ---- 快捷操作 ----
function quickAction(action) {
  switch (action) {
    case 'start':
      appendCmd('> 正在检查全部机器人…', 'ok');
      runCommand('status');
      break;
    case 'pause':
      appendCmd('> 正在暂停全部机器人…', 'ok');
      runCommand('pause');
      break;
    case 'resume':
      appendCmd('> 正在恢复全部机器人…', 'ok');
      runCommand('resume');
      break;
    case 'redeem':
      appendCmd('🎁 请在上方命令栏输入兑换码：!redeem <code>', 'ok');
      break;
  }
  switchPage('commands');
}

async function botAction(bot, action) {
  if (action === 'start' || action === 'pause' || action === 'resume') {
    appendCmd('> ' + bot + ' ' + action + ' …', 'ok');
    await runCommand(action + ' ' + bot);
  } else {
    appendCmd('> ' + bot + ' ' + action + ' … 已发送', 'ok');
  }
}

function addBot() {
  const list = document.getElementById('bot-list');
  const n = document.querySelectorAll('.bot-card').length + 1;
  const div = document.createElement('div');
  div.className = 'bot-card';
  div.innerHTML = `
    <div class="bot-avatar">B${n}</div>
    <div class="bot-info"><div class="bot-name">newbot${n}</div><div class="bot-sub">在线 · 空闲</div></div>
    <div class="bot-status online">在线</div>`;
  list.appendChild(div);
  document.getElementById('bot-count').textContent = n;
}

// ---- 机器人列表刷新（从 /api/asf 读取真实数据）----
async function refreshBots() {
  const base = getBase();
  const pass = getPass();
  try {
    const resp = await fetch(base + '/api/asf', {
      headers: { 'Authentication': pass }
    });
    if (!resp.ok) return;
    const data = await resp.json();
    const bots = data.Result && data.Result.Bots ? data.Result.Bots : null;
    const countEl = document.getElementById('bot-count');
    const listEl = document.getElementById('bot-list');
    if (!bots || typeof bots !== 'object') return;
    const names = Object.keys(bots);
    if (countEl) countEl.textContent = names.length;
    // 更新已存在的卡片状态
    names.forEach(name => {
      const info = bots[name];
      const statusEl = document.getElementById('bot-status-' + name);
      const subEl = document.getElementById('bot-sub-' + name);
      if (!statusEl) return;
      const connected = info.IsConnectedAndLoggedOn;
      const farming = info.CardsFarmer && info.CardsFarmer.NowFarming;
      if (farming) {
        statusEl.textContent = '挂机中';
        statusEl.className = 'bot-status farming';
      } else if (connected) {
        statusEl.textContent = '在线';
        statusEl.className = 'bot-status online';
      } else {
        statusEl.textContent = '离线';
        statusEl.className = 'bot-status offline';
      }
      if (subEl) {
        subEl.textContent = farming ? '挂机中 · 正在掉落卡牌' : (connected ? '在线 · 空闲' : '未连接');
      }
    });
  } catch (e) {
    // 忽略：连接失败时不打扰用户
  }
}

// ---- 命令输出 ----
function appendCmd(text, type) {
  const out = document.getElementById('cmd-output');
  const parts = String(text).split('\n');
  const div = document.createElement('div');
  div.className = 'cmd-line ' + (type || '');
  div.textContent = '> ' + parts[0];
  out.appendChild(div);
  for (let i = 1; i < parts.length; i++) {
    const r = document.createElement('div');
    r.className = 'cmd-result';
    r.textContent = parts[i];
    out.appendChild(r);
  }
  out.scrollTop = out.scrollHeight;
}

function toggleDark(el) {
  document.body.style.background = el.checked
    ? 'linear-gradient(135deg, #0a0c10, #1a2332)'
    : 'linear-gradient(135deg, #e8edf3, #cfd8e6)';
}
