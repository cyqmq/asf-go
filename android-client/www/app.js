(function () {
  "use strict";

  var serviceRunning = false;
  var demoMode = false; // 连接失败时进入演示模式，保留示例数据

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
    return {
      host: (document.getElementById("server-host") || {}).value || "127.0.0.1",
      port: (document.getElementById("server-port") || {}).value || "1242",
      password: (document.getElementById("server-password") || {}).value || ""
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
    } catch (e) { /* 忽略损坏配置 */ }
  }

  /* ─── 连接与状态 ─── */
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

  async function refreshStatus() {
    try {
      var data = await fetchAsf("/api/asf");
      var bots = data.Result && data.Result.Bots ? data.Result.Bots : {};
      var names = Object.keys(bots);
      var farming = 0;
      names.forEach(function (name) {
        var b = bots[name];
        if (b && b.CardsFarmer && b.CardsFarmer.NowFarming) farming++;
      });
      setVal("bots-val", names.length + " / " + names.length);
      setVal("farming-val", String(farming));
      setVal("info-bots", String(names.length));
      setVal("info-farming", String(farming));
      setVal("info-cards", "—");
      setVal("info-trades", "—");
      renderBotRows(names, bots);
      addLog("INFO", "已刷新状态：" + names.length + " 个机器人，" + farming + " 个挂机中");
      return true;
    } catch (e) {
      addLog("ERROR", "刷新状态失败: " + e.message);
      return false;
    }
  }

  function renderBotRows(names, bots) {
    var container = document.getElementById("bot-list");
    if (!container) return;
    var template = container.querySelector(".bot-row");
    if (!template) return;
    names.forEach(function (name) {
      var b = bots[name] || {};
      var connected = b.IsConnectedAndLoggedOn;
      var farming = b.CardsFarmer && b.CardsFarmer.NowFarming;
      var row = document.getElementById("bot-row-" + name);
      if (!row) {
        // 克隆模板创建新行
        row = template.cloneNode(true);
        row.id = "bot-row-" + name;
        row.removeAttribute("id");
        container.appendChild(row);
      }
      // 更新 onclick 指向真实 bot 名
      row.setAttribute("onclick", "toggleBotDetail('" + name + "')");
      // 头像
      var avatar = row.querySelector(".bot-avatar");
      if (avatar) avatar.textContent = name.charAt(0).toUpperCase();
      // 名称与状态
      var nameEl = row.querySelector(".bot-name");
      var statusEl = nameEl ? nameEl.querySelector(".bot-status") : null;
      if (nameEl) {
        // 保留首位文本节点为 bot 名，替换掉旧名
        var textNode = nameEl.childNodes[0];
        if (textNode) textNode.textContent = name + " ";
        else nameEl.insertBefore(document.createTextNode(name + " "), nameEl.firstChild);
        if (statusEl) {
          statusEl.textContent = farming ? "挂机中" : (connected ? "在线" : "离线");
          statusEl.className = "bot-status " + (farming ? "farming" : (connected ? "online" : "offline"));
        }
      }
      // 副标题
      var subEl = row.querySelector(".bot-sub");
      if (subEl) {
        subEl.textContent = farming ? "正在掉落卡牌" : (connected ? "空闲" : "未连接");
      }
      row.setAttribute("data-status", farming ? "farming" : (connected ? "online" : "offline"));
      // 动作按钮
      row.querySelectorAll(".bot-act").forEach(function (btn) {
        var action = btn.getAttribute("onclick") || "";
        btn.setAttribute("onclick", "botAction('" + name + "','" + (action.indexOf("pause") >= 0 ? "暂停" : action.indexOf("resume") >= 0 ? "恢复" : "启动") + "')");
      });
      // 详情区
      var detail = document.getElementById("detail-" + name);
      if (!detail) {
        detail = document.createElement("div");
        detail.className = "bot-detail hidden";
        detail.id = "detail-" + name;
        detail.innerHTML = '<div class="detail-row"><span>SteamID</span><span class="mono">' + (b.s_SteamID || "—") + '</span></div>' +
          '<div class="detail-row"><span>挂机中</span><span>' + (farming ? "是" : "否") + '</span></div>' +
          '<div class="detail-row"><span>KeepRunning</span><span>' + (b.KeepRunning ? "是" : "否") + '</span></div>';
        row.after(detail);
      }
    });
  }

  /* ─── 内置服务端（Capacitor 原生插件） ─── */
  function hasNativePlugin() {
    return !!(window.Capacitor && window.Capacitor.Plugins && window.Capacitor.Plugins.AsfGo);
  }

  async function ensureLocalServer() {
    if (!hasNativePlugin()) return false;
    try {
      var pass = getSettings().password || "testpass";
      await window.Capacitor.Plugins.AsfGo.start({ password: pass });
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
    if (!serviceRunning) {
      var localStarted = await ensureLocalServer();
      var ok = false;
      if (localStarted) {
        // 内置服务需要时间初始化，轮询等待 IPC 就绪
        for (var i = 0; i < 8; i++) {
          await new Promise(function (r) { setTimeout(r, 1000); });
          try {
            var data = await fetchAsf("/api/asf");
            if (data && data.Success) { ok = true; break; }
          } catch (e) { /* 尚未就绪，继续等待 */ }
        }
        demoMode = !ok;
        if (ok) {
          addLog("INFO", "内置 asf-go 服务端就绪，IPC 连接成功");
        } else {
          addLog("WARN", "内置服务已启动但 IPC 未就绪，进入演示模式");
        }
      } else {
        // 浏览器环境：尝试远程连接
        try {
          var data2 = await fetchAsf("/api/asf");
          ok = !!(data2 && data2.Success);
          demoMode = !ok;
        } catch (e) {
          ok = false;
          demoMode = true;
        }
        if (!ok) {
          addLog("WARN", "无法连接 asf-go，已进入演示模式（可修改设置页地址后重试）");
        }
      }
    } else {
      if (hasNativePlugin()) {
        await stopLocalServer();
      }
    }
    serviceRunning = !serviceRunning;
    updateServiceUI();
    if (serviceRunning && !demoMode) {
      refreshStatus();
    }
  };

  function updateServiceUI() {
    var btn = document.getElementById("launch-btn");
    var icon = document.getElementById("launch-icon");
    var text = document.getElementById("launch-text");
    var pill = document.getElementById("status-pill");
    var pillDot = document.getElementById("pill-dot");
    var label = document.getElementById("status-label");
    var hint = document.getElementById("hint-text");
    var tip = document.getElementById("tip-text");
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
      hint.textContent = demoMode
        ? "演示模式：已连接模拟数据。可在设置页配置真实 asf-go 服务器后重新启动。"
        : "已连接 asf-go，可前往控制台发送 status / pause / resume 等命令管理挂卡。";
      tip.textContent = demoMode
        ? "当前为演示数据。配置服务器地址、端口和 IPC 密码后可管理真实挂卡机器人。"
        : "已连接 asf-go，可在 Web 管理界面查看 ASF-ui。";
      led.classList.add("running");
      statusVal.textContent = demoMode ? "demo" : "running";
      statusVal.style.color = "#4caf50";
      modeRow.innerHTML = '<svg class="icon-16"><use href="#i-globe"/></svg>';
      modeText.textContent = demoMode ? "演示模式" : "公开模式";
      if (demoMode) {
        setVal("bots-val", "2 / 3");
        setVal("farming-val", "1");
        setVal("info-bots", "2");
        setVal("info-farming", "1");
        setVal("info-cards", "5");
        setVal("info-trades", "1");
      }
      addLog("INFO", demoMode ? "已启动演示模式" : "已连接 asf-go，服务运行中");
      addConsoleMsg("bot", demoMode
        ? "已进入演示模式。可在设置页配置真实 asf-go 服务器（地址/端口/IPC 密码）后重新启动。"
        : "已连接 asf-go。当前所有机器人状态正常。");
    } else {
      btn.classList.remove("running");
      icon.innerHTML = '<use href="#i-play-fill"/>';
      text.textContent = "启动 ASF 服务";
      pill.className = "status-pill";
      pillDot.className = "pill-dot";
      label.textContent = "空闲";
      hint.textContent = "asf-go 是 ArchiSteamFarm 的 Go 语言实现，连接后可管理挂卡机器人、发送命令并查看日志。";
      tip.textContent = "asf-go 是 ArchiSteamFarm 的 Go 语言实现，连接后可查看机器人状态、发送命令、查看日志。";
      led.classList.remove("running");
      statusVal.textContent = "stopped";
      statusVal.style.color = "";
      modeRow.innerHTML = '<svg class="icon-16"><use href="#i-lock"/></svg>';
      modeText.textContent = "本地模式";
      setVal("bots-val", "0 / 0");
      setVal("farming-val", "0");
      setVal("info-bots", "0");
      setVal("info-farming", "0");
      setVal("info-cards", "0");
      setVal("info-trades", "0");
      addLog("INFO", "已断开与 asf-go 的连接");
      addConsoleMsg("bot", "已断开连接。点击仪表盘「启动 ASF 服务」重新连接。");
    }
  }

  function setVal(id, v) {
    var el = document.getElementById(id);
    if (el) el.textContent = v;
  }

  /* ─── 端点点击 ─── */
  window.openEndpoint = function () {
    toastMsg("已在浏览器打开端点地址");
  };

  /* ─── 机器人管理 ─── */
  window.refreshBots = function () {
    toastMsg("正在刷新机器人列表…");
    if (serviceRunning && !demoMode) {
      refreshStatus();
    } else {
      addLog("WARN", "服务未连接，仅刷新演示数据");
      toastMsg("演示模式：未连接真实服务");
    }
  };

  window.botAction = function (name, action) {
    // action 可能是中文（暂停/恢复/启动）或英文
    var cmdMap = { "暂停": "pause", "恢复": "resume", "启动": "start", "挂机": "farm" };
    var en = cmdMap[action] || action;
    addLog("INFO", "发送命令: !" + en + " " + name);
    if (serviceRunning && !demoMode) {
      addConsoleMsg("user", "!" + en + " " + name);
      runCommand(en + " " + name);
    } else {
      addConsoleMsg("user", "!" + en + " " + name);
      setTimeout(function () {
        addConsoleMsg("bot", "已发送：" + name + " " + action + "（演示模式）");
      }, 300);
      toastMsg("演示模式：" + name + " " + action);
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
    if (serviceRunning && !demoMode) {
      runCommand(text);
    } else {
      setTimeout(function () {
        addConsoleMsg("bot", buildReply(text));
      }, 300);
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

  function buildReply(cmd) {
    var c = String(cmd).toLowerCase();
    if (c.indexOf("status") >= 0) {
      return "testbot  | 挂机中 | TF2 (440) | 剩余 5 卡\ntestbot2 | 在线 | 空闲\noldbot   | 离线 | 上次在线 3 天前";
    }
    if (c.indexOf("stats") >= 0) {
      return "全局: 在线 2 / 总数 3 · 挂机中 1 · 今日掉卡 3 张";
    }
    if (c.indexOf("version") >= 0) {
      return "ASF-Go v0.5.0 (Go 语言实现)";
    }
    if (c.indexOf("pause") >= 0) {
      return "已暂停: testbot";
    }
    if (c.indexOf("resume") >= 0) {
      return "已恢复: testbot";
    }
    if (c.indexOf("help") >= 0) {
      return "可用: status / stats / pause <bot> / resume <bot> / redeem / version / help";
    }
    return "未知命令: " + cmd + "（输入 help 查看帮助）";
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
