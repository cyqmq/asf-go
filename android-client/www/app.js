(function () {
  "use strict";

  var serviceRunning = false;

  /* ─── 页面切换 ─── */
  window.switchPage = function (page) {
    document.querySelectorAll(".page").forEach(function (el) { el.classList.remove("active"); });
    document.getElementById("page-" + page).classList.add("active");
    document.querySelectorAll(".nav-btn").forEach(function (el) {
      el.classList.toggle("active", el.getAttribute("data-page") === page);
    });
  };

  /* ─── 主题切换 ─── */
  window.setTheme = function (theme) {
    document.body.setAttribute("data-theme", theme);
    document.querySelectorAll(".theme-pill").forEach(function (el) {
      el.classList.toggle("active", el.getAttribute("data-theme") === theme);
    });
  };

  /* ─── 服务器配置 ─── */
  function getSettings() {
    var pm = document.getElementById("public-mode");
    return {
      host: (document.getElementById("server-host") || {}).value || "127.0.0.1",
      port: (document.getElementById("server-port") || {}).value || "1242",
      password: (document.getElementById("server-password") || {}).value || "",
      publicMode: pm ? pm.checked : false
    };
  }
  function getBase() {
    var s = getSettings();
    return "http://" + s.host + ":" + s.port;
  }
  function getAuth() {
    return getSettings().password;
  }

  window.saveSettings = function () {
    try {
      localStorage.setItem("asfgo-server", JSON.stringify(getSettings()));
      applyPublicMode();
      toastMsg("配置已保存");
      addLog("INFO", "服务器配置已保存");
    } catch (e) {
      toastMsg("保存失败");
    }
  };

  function loadSettings() {
    try {
      var raw = localStorage.getItem("asfgo-server");
      if (!raw) return;
      var s = JSON.parse(raw);
      if (document.getElementById("server-host")) document.getElementById("server-host").value = s.host || "127.0.0.1";
      if (document.getElementById("server-port")) document.getElementById("server-port").value = s.port || "1242";
      if (document.getElementById("server-password")) document.getElementById("server-password").value = s.password || "";
      var pm = document.getElementById("public-mode");
      if (pm) pm.checked = !!s.publicMode;
      applyPublicMode();
    } catch (e) { /* 忽略损坏配置 */ }
  }

  /* 公共模式：开启后地址栏禁用（固定连接本地 127.0.0.1），服务监听 0.0.0.0 允许外部访问 */
  function applyPublicMode() {
    var pm = document.getElementById("public-mode");
    var host = document.getElementById("server-host");
    if (!pm || !host) return;
    host.disabled = pm.checked;
    if (pm.checked) {
      host.value = "127.0.0.1";
    }
  }

  window.togglePublicMode = function (el) {
    applyPublicMode();
    saveSettings();
    toastMsg(el.checked ? "公共模式已开启：服务将允许外部访问" : "公共模式已关闭");
  };

  /* ─── IPC 请求 ─── */
  async function fetchAsf(path, options) {
    var base = getBase();
    var pass = getAuth();
    var opts = options || {};
    opts.headers = Object.assign({ "Authentication": pass }, opts.headers || {});
    var resp = await fetch(base + path, opts);
    if (!resp.ok) throw new Error("HTTP " + resp.status);
    return resp.json();
  }

  window.testConnection = async function () {
    try {
      var data = await fetchAsf("/api/asf");
      var ver = data.Result && data.Result.Version ? data.Result.Version : "?";
      toastMsg("连接成功 v" + ver);
      addLog("INFO", "连接 asf-go " + getBase() + " 成功 (v" + ver + ")");
      return true;
    } catch (e) {
      toastMsg("连接失败: " + e.message);
      addLog("ERROR", "连接 asf-go " + getBase() + " 失败: " + e.message);
      return false;
    }
  };

  /* ─── 内置服务端（Capacitor 原生插件） ─── */
  function hasNativePlugin() {
    return !!(window.Capacitor && window.Capacitor.Plugins && window.Capacitor.Plugins.AsfGo);
  }

  async function ensureLocalServer() {
    if (!hasNativePlugin()) return false;
    try {
      var pass = getSettings().password || "testpass";
      await window.Capacitor.Plugins.AsfGo.start({ password: pass, public: getSettings().publicMode });
      addLog("INFO", "已启动内置 asf-go 服务端 (127.0.0.1:1242)");
      return true;
    } catch (e) {
      addLog("ERROR", "启动内置 asf-go 失败: " + e.message);
      return false;
    }
  }

  async function stopLocalServer() {
    if (!hasNativePlugin()) return;
    try {
      await window.Capacitor.Plugins.AsfGo.stop();
      addLog("INFO", "已停止内置 asf-go 服务端");
    } catch (e) {
      addLog("WARN", "停止内置服务失败: " + e.message);
    }
  }

  /* ─── 服务启动/停止 ─── */
  window.toggleService = async function () {
    if (serviceRunning) {
      await stopLocalServer();
      serviceRunning = false;
      updateServiceUI();
      addLog("INFO", "已停止服务");
      return;
    }
    addLog("INFO", "正在启动服务…");
    var localStarted = await ensureLocalServer();
    var ok = false;
    var attempts = localStarted ? 8 : 1;
    for (var i = 0; i < attempts; i++) {
      if (localStarted) {
        await new Promise(function (r) { setTimeout(r, 1000); });
      }
      try {
        var data = await fetchAsf("/api/asf");
        if (data && data.Success) { ok = true; break; }
      } catch (e) { /* 未就绪，重试 */ }
    }
    if (ok) {
      serviceRunning = true;
      updateServiceUI();
      updateEndpoint();
      if (localStarted) {
        addLog("INFO", "内置 asf-go 服务端就绪，IPC 连接成功");
      } else {
        addLog("INFO", "已连接 asf-go " + getBase());
      }
      refreshStatus();
    } else {
      updateServiceUI();
      if (localStarted) {
        addLog("ERROR", "内置服务已启动但 IPC 未就绪，请查看日志");
      } else {
        addLog("ERROR", "无法连接 asf-go " + getBase() + "，请检查地址/端口/密码");
      }
      toastMsg("连接失败，请检查设置");
    }
  };

  function updateServiceUI() {
    var btn = document.getElementById("launch-btn");
    var icon = document.getElementById("launch-icon");
    var text = document.getElementById("launch-text");
    var pill = document.getElementById("status-pill");
    var pillDot = document.getElementById("pill-dot");
    var label = document.getElementById("status-label");
    var led = document.getElementById("status-led");
    var statusVal = document.getElementById("status-val");
    var modeRow = document.getElementById("mode-row");
    var modeText = document.getElementById("mode-text");

    if (serviceRunning) {
      btn.classList.add("running");
      icon.innerHTML = '<use href="#i-stop-fill"/>';
      text.textContent = "停止服务";
      pill.className = "status-pill running";
      pillDot.className = "pill-dot glow";
      label.textContent = "运行中";
      led.classList.add("running");
      statusVal.textContent = "running";
      statusVal.style.color = "#4caf50";
      modeRow.innerHTML = '<svg class="icon-16"><use href="#i-globe"/></svg>';
      modeText.textContent = "公开模式";
    } else {
      btn.classList.remove("running");
      icon.innerHTML = '<use href="#i-play-fill"/>';
      text.textContent = "启动 ASF 服务";
      pill.className = "status-pill";
      pillDot.className = "pill-dot";
      label.textContent = "空闲";
      led.classList.remove("running");
      statusVal.textContent = "stopped";
      statusVal.style.color = "";
      modeRow.innerHTML = '<svg class="icon-16"><use href="#i-lock"/></svg>';
      modeText.textContent = "本地模式";
    }
  }

  function setVal(id, v) {
    var el = document.getElementById(id);
    if (el) el.textContent = v;
  }

  /* ─── 状态刷新 ─── */
  async function refreshStatus() {
    try {
      var data = await fetchAsf("/api/asf");
      var bots = data.Result && data.Result.Bots ? data.Result.Bots : {};
      var names = Object.keys(bots);
      var farming = 0, online = 0;
      names.forEach(function (name) {
        var b = bots[name];
        if (b && b.IsConnectedAndLoggedOn) online++;
        if (b && b.CardsFarmer && b.CardsFarmer.NowFarming) farming++;
      });
      var offline = names.length - online;
      setVal("sum-online", String(online));
      setVal("sum-farming", String(farming));
      setVal("sum-offline", String(offline));
      renderBotRows(names, bots);
      addLog("INFO", "已刷新状态：" + names.length + " 个机器人，" + farming + " 个挂机中");
      return true;
    } catch (e) {
      addLog("ERROR", "刷新状态失败: " + e.message);
      return false;
    }
  }

  function renderBotRows(names, bots) {
    var container = document.querySelector("#page-web .bots-body");
    if (!container) return;
    // 移除旧的动态行（保留 summary 和添加按钮）
    container.querySelectorAll(".bot-row, .bot-detail").forEach(function (el) { el.remove(); });
    if (names.length === 0) {
      var empty = document.createElement("div");
      empty.className = "empty-tip";
      empty.textContent = "暂无机器人。请在服务器端创建 Bot 配置文件。";
      container.insertBefore(empty, container.querySelector(".add-bot"));
      return;
    }
    names.forEach(function (name) {
      var b = bots[name] || {};
      var connected = b.IsConnectedAndLoggedOn;
      var farming = b.CardsFarmer && b.CardsFarmer.NowFarming;
      var row = document.createElement("div");
      row.className = "bot-row";
      row.setAttribute("data-status", farming ? "farming" : (connected ? "online" : "offline"));
      row.setAttribute("onclick", "toggleBotDetail('" + name + "')");
      row.innerHTML =
        '<div class="bot-avatar" style="background:linear-gradient(135deg,#2b6cb0,#66c0f4)">' + name.charAt(0).toUpperCase() + '</div>' +
        '<div class="bot-main">' +
        '  <div class="bot-name">' + name + ' <span class="bot-status ' + (farming ? "farming" : (connected ? "online" : "offline")) + '">' + (farming ? "挂机中" : (connected ? "在线" : "离线")) + '</span></div>' +
        '  <div class="bot-sub">' + (farming ? "正在掉落卡牌" : (connected ? "空闲" : "未连接")) + '</div>' +
        '</div>' +
        '<div class="bot-actions" onclick="event.stopPropagation()">' +
        '  <button class="bot-act" onclick="botAction(\'' + name + '\',\'暂停\')" title="暂停"><svg class="icon-14"><use href="#i-pause-fill"/></svg></button>' +
        '  <button class="bot-act primary" onclick="botAction(\'' + name + '\',\'恢复\')" title="恢复"><svg class="icon-14"><use href="#i-play-fill"/></svg></button>' +
        '</div>';
      var detail = document.createElement("div");
      detail.className = "bot-detail hidden";
      detail.id = "detail-" + name;
      detail.innerHTML =
        '<div class="detail-row"><span>SteamID</span><span class="mono">' + (b.s_SteamID || "—") + '</span></div>' +
        '<div class="detail-row"><span>挂机中</span><span>' + (farming ? "是" : "否") + '</span></div>' +
        '<div class="detail-row"><span>KeepRunning</span><span>' + (b.KeepRunning ? "是" : "否") + '</span></div>';
      container.insertBefore(row, container.querySelector(".add-bot"));
      container.insertBefore(detail, container.querySelector(".add-bot"));
    });
  }

  /* ─── 端点点击 ─── */
  window.openEndpoint = function () {
    toastMsg("已在浏览器打开端点地址");
  };

  /* ─── 机器人管理 ─── */
  window.refreshBots = function () {
    if (!serviceRunning) {
      toastMsg("请先启动服务");
      return;
    }
    toastMsg("正在刷新机器人列表…");
    refreshStatus();
  };

  window.botAction = function (name, action) {
    var cmdMap = { "暂停": "pause", "恢复": "resume", "启动": "start", "挂机": "farm" };
    var en = cmdMap[action] || action;
    addLog("INFO", "发送命令: !" + en + " " + name);
    if (serviceRunning) {
      addConsoleMsg("user", "!" + en + " " + name);
      runCommand(en + " " + name);
    } else {
      toastMsg("请先启动服务");
    }
  };

  window.toggleBotDetail = function (name) {
    var el = document.getElementById("detail-" + name);
    if (el) el.classList.toggle("hidden");
  };

  /* ─── 命令控制台 ─── */
  window.sendCommand = function () {
    var input = document.getElementById("console-input");
    var text = input.value.trim();
    if (!text) return;
    addConsoleMsg("user", "!" + text);
    input.value = "";
    if (serviceRunning) {
      runCommand(text);
    } else {
      addConsoleMsg("bot", "服务未连接，请先启动服务。");
    }
  };

  async function runCommand(cmd) {
    try {
      var data = await fetchAsf("/api/command", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: cmd })
      });
      var msg = data.Message || data.Result || (data.Success ? "OK" : "命令失败");
      addConsoleMsg("bot", String(msg));
      addLog(data.Success ? "INFO" : "WARN", "命令 !" + cmd + (data.Success ? " 执行成功" : " 执行失败"));
    } catch (e) {
      addConsoleMsg("bot", "请求失败: " + e.message);
      addLog("ERROR", "命令请求失败: " + e.message);
    }
  }

  function addConsoleMsg(role, text) {
    var body = document.getElementById("console-body");
    if (!body) return;
    var div = document.createElement("div");
    div.className = "msg " + role;
    if (role === "bot") {
      div.innerHTML = '<div class="avatar"><svg class="icon-16"><use href="#i-server"/></svg></div><div class="bubble mono"></div>';
    } else {
      div.innerHTML = '<div class="bubble"></div>';
    }
    var bubble = div.querySelector(".bubble");
    bubble.textContent = text;
    body.appendChild(div);
    body.scrollTop = body.scrollHeight;
  }

  /* ─── 日志 ─── */
  function addLog(level, msg) {
    var container = document.getElementById("log-container");
    if (!container) return;
    var now = new Date();
    var time = pad(now.getHours()) + ":" + pad(now.getMinutes()) + ":" + pad(now.getSeconds());
    var line = document.createElement("div");
    line.className = "log-line " + level.toLowerCase();
    line.innerHTML = '<span class="log-time">' + time + '</span><span class="log-lvl ' + level + '">' + level + '</span><span></span>';
    line.lastElementChild.textContent = msg;
    container.appendChild(line);
    container.scrollTop = container.scrollHeight;
  }
  function pad(n) { return n < 10 ? "0" + n : "" + n; }

  window.downloadLog = function () {
    var lines = document.querySelectorAll(".log-line");
    var text = Array.from(lines).map(function (l) { return l.textContent; }).join("\n");
    var blob = new Blob([text], { type: "text/plain" });
    var url = URL.createObjectURL(blob);
    var a = document.createElement("a");
    a.href = url;
    a.download = "asf-go.log";
    a.click();
    setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
  };

  window.logToggle = function (el, name) {
    addLog(el.checked ? "INFO" : "WARN", (el.checked ? "已开启 " : "已关闭 ") + name);
  };

  /* ─── 关于弹窗 ─── */
  window.showAbout = function () {
    document.getElementById("about-modal").classList.remove("hidden");
  };
  window.hideAbout = function () {
    document.getElementById("about-modal").classList.add("hidden");
  };

  /* ─── 添加机器人 ─── */
  window.showAddBot = function () {
    document.getElementById("addbot-modal").classList.remove("hidden");
    var input = document.getElementById("new-bot-name");
    if (input) input.focus();
  };
  window.hideAddBot = function () {
    document.getElementById("addbot-modal").classList.add("hidden");
  };
  function val(id) {
    var el = document.getElementById(id);
    return el ? el.value.trim() : "";
  }
  function checked(id) {
    var el = document.getElementById(id);
    return el ? el.checked : false;
  }

  window.confirmAddBot = async function () {
    var input = document.getElementById("new-bot-name");
    var name = input ? input.value.trim() : "";
    if (!name) { toastMsg("请输入机器人名称"); return; }
    if (!serviceRunning) { toastMsg("请先启动服务"); return; }
    var botConfig = {
      Enabled: checked("cfg-enabled"),
      SteamLogin: val("cfg-login"),
      SteamPassword: val("cfg-password"),
      SteamUserPermissions: {},
      OnlineStatus: val("cfg-online-status") || "Online",
      RemoteCommunication: val("cfg-remote-comm") || "All",
      PasswordFormat: "PlainText",
      HoursUntilCardDrops: parseInt(val("cfg-hours") || "3", 10),
      CustomGamePlayedWhileFarming: val("cfg-custom-game"),
      Trading: checked("cfg-trading"),
      TradingSteamLevel: parseInt(val("cfg-trading-level") || "5", 10),
      TradeCheckPeriod: parseInt(val("cfg-trade-check") || "60", 10),
      TradingPreferences: parseInt(val("cfg-trading-prefs") || "0", 10),
      SteamParentalCode: val("cfg-parental"),
      SteamMasterClanID: parseInt(val("cfg-clan") || "0", 10) || 0,
      UseLoginKeys: checked("cfg-use-login-keys")
    };
    try {
      var data = await fetchAsf("/api/bot/" + encodeURIComponent(name), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ botConfig: botConfig })
      });
      if (data.Success) {
        toastMsg("已创建机器人 " + name);
        addLog("INFO", "已创建机器人 " + name);
        if (input) input.value = "";
        hideAddBot();
        refreshStatus();
      } else {
        toastMsg("创建失败: " + (data.Message || "未知错误"));
      }
    } catch (e) {
      toastMsg("创建失败: " + e.message);
    }
  };

  /* 公共模式下显示设备局域网 IP */
  async function updateEndpoint() {
    var el = document.getElementById("endpoint-url");
    if (!el) return;
    if (getSettings().publicMode && hasNativePlugin()) {
      try {
        var res = await window.Capacitor.Plugins.AsfGo.getLocalIp();
        el.textContent = "http://" + (res.ip || "0.0.0.0") + ":1242";
        return;
      } catch (e) { /* 回退 */ }
    }
    el.textContent = "http://127.0.0.1:1242";
  }

  /* ─── Toast ─── */
  var toastTimer = null;
  window.toastMsg = function (msg) {
    var t = document.getElementById("toast");
    if (!t) return;
    t.textContent = msg;
    t.classList.add("show");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.classList.remove("show"); }, 1800);
  };

  /* 初始化 */
  loadSettings();
})();
