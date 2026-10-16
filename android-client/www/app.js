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

  /* ─── 服务启动/停止 ─── */
  window.toggleService = function () {
    serviceRunning = !serviceRunning;
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
      hint.textContent = "已连接 asf-go，可前往控制台发送 status / pause / resume 等命令管理挂卡。";
      tip.textContent = "已连接 asf-go，2 个机器人在线，1 个挂机中。可在 Web 管理界面查看 ASF-ui。";
      led.classList.add("running");
      statusVal.textContent = "running";
      statusVal.style.color = "#4caf50";
      modeRow.innerHTML = '<svg class="icon-16"><use href="#i-globe"/></svg>';
      modeText.textContent = "公开模式";
      setVal("bots-val", "2 / 3");
      setVal("farming-val", "1");
      setVal("info-bots", "2");
      setVal("info-farming", "1");
      setVal("info-cards", "5");
      setVal("info-trades", "1");
      addLog("INFO", "已连接 asf-go v0.5.0 (127.0.0.1:1242)");
      addLog("INFO", "testbot 开始挂机 Team Fortress 2 (440)");
      addLog("WARN", "oldbot 离线：未配置 Steam 凭据");
      addConsoleMsg("bot", "已连接 asf-go v0.5.0。当前 2 个机器人在线，1 个挂机中。");
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
  };

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
    toastMsg("已刷新机器人列表");
    addLog("INFO", "已刷新机器人列表");
  };

  window.botAction = function (name, action) {
    toastMsg("已发送：" + name + " " + action);
    addLog("INFO", name + " " + action + " 已执行");
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
    setTimeout(function () {
      addConsoleMsg("bot", buildReply(text));
    }, 400);
    input.value = "";
  };

  function buildReply(cmd) {
    var c = cmd.toLowerCase();
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
    t.textContent = msg;
    t.classList.add("show");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.classList.remove("show"); }, 1800);
  };
})();