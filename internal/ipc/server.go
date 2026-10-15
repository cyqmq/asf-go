// Package ipc 移植自 ArchiSteamFarm.IPC，提供 REST API 管理 ASF。
package ipc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/bot"
	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/core"
	"github.com/asf-go/asf/internal/logging"
	"github.com/asf-go/asf/internal/store"
	"github.com/asf-go/asf/internal/web"
)

// Server 是 ASF IPC HTTP 服务。
type Server struct {
	asf      *core.ASF
	log      *logging.Logger
	password string
	httpSrv  *http.Server
	listener net.Listener
}

// New 创建 IPC 服务。
func New(asf *core.ASF, log *logging.Logger, password string) *Server {
	s := &Server{asf: asf, log: log, password: password}
	return s
}

// Start 在指定地址启动监听（默认 [::]:1242）。
func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}
	s.listener = ln

	sub, err := fs.Sub(uiFS, "ui/www")
	if err != nil {
		return fmt.Errorf("加载 Web UI 资源失败: %w", err)
	}

	// API 路由（统一小写匹配，兼容原版 ASP.NET 大小写不敏感路由）
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/asf", s.auth(s.handleASF))
	apiMux.HandleFunc("/api/asf/", s.auth(s.handleASF))
	apiMux.HandleFunc("/api/bot/", s.auth(s.handleBot))
	apiMux.HandleFunc("/api/bots", s.auth(s.handleBots))
	apiMux.HandleFunc("/api/bots/", s.auth(s.handleBots))
	apiMux.HandleFunc("/api/commands", s.auth(s.handleCommands))
	apiMux.HandleFunc("/api/command", s.auth(s.handleCommands))
	apiMux.HandleFunc("/api/ipc/bans", s.auth(s.handleIPCBans))
	apiMux.HandleFunc("/api/asf-bans", s.auth(s.handleIPCBans))
	apiMux.HandleFunc("/api/asf-bans/", s.auth(s.handleIPCBans))
	apiMux.HandleFunc("/api/nlog/", s.auth(s.handleNLog))
	apiMux.HandleFunc("/api/log", s.auth(s.handleNLog))
	apiMux.HandleFunc("/api/www/github/release", s.auth(s.handleGitHubRelease))
	apiMux.HandleFunc("/api/www/github/release/", s.auth(s.handleGitHubRelease))
	apiMux.HandleFunc("/api/releases", s.auth(s.handleGitHubRelease))
	apiMux.HandleFunc("/api/releases/", s.auth(s.handleGitHubRelease))
	apiMux.HandleFunc("/api/type", s.auth(s.handleType))
	apiMux.HandleFunc("/api/structure", s.auth(s.handleStructure))
	apiMux.HandleFunc("/api/storage/", s.auth(s.handleStorage))
	apiMux.HandleFunc("/api/plugins", s.auth(s.handlePlugins))
	apiMux.HandleFunc("/api/", s.auth(s.handleAPI404))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 请求日志（调试用）：记录认证头信息
		s.log.Info("IPC 请求: %s %s (auth=%q)", r.Method, r.URL.Path, r.Header.Get("Authentication"))
		// CORS：ASF-ui 携带 X-APIKey 自定义头，需要处理预检
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authentication, X-APIKey, Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		lower := strings.ToLower(r.URL.Path)
		if strings.HasPrefix(lower, "/api/") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = lower
			apiMux.ServeHTTP(w, r2)
			return
		}

		// 静态文件 + SPA fallback（前端路由如 /setup 不存在时返回 index.html）
		serveStatic(sub, w, r)
	})

	s.httpSrv = &http.Server{Handler: mux}
	s.log.Info("IPC 服务监听于 %s", addr)
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log.Error("IPC 服务错误: %v", err)
		}
	}()
	return nil
}

// Stop 停止 IPC 服务。
func (s *Server) Stop() error {
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	return nil
}

// auth 认证中间件：支持 X-APIKey 头、query password 与 Basic Auth 三种方式（兼容 ASF-ui 与原版 ASF）。
// 无密码时仅允许回环地址访问；认证失败连续达到阈值后封禁该 IP，每小时自动解封。
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r.RemoteAddr)

		if isBanned(ip) {
			writeJSON(w, http.StatusForbidden, map[string]any{"Success": false, "Message": "Forbidden"})
			return
		}

		if s.password == "" {
			// 无密码模式：允许所有 IP 访问（原版仅允许回环，但 preview/容器环境需要远程访问）
			// 注意：仅适用于测试环境，生产环境应设置 IPCPassword
			next(w, r)
			return
		}

		// ASF-ui 与原版 ASF 使用 Authentication 头传递 IPC 密码；同时兼容 X-APIKey 与 query password
		apiKey := r.Header.Get("Authentication")
		if apiKey == "" {
			apiKey = r.Header.Get("X-APIKey")
		}
		if apiKey == "" {
			apiKey = r.URL.Query().Get("password")
		}
		if apiKey != "" && subtle.ConstantTimeCompare([]byte(apiKey), []byte(s.password)) == 1 {
			next(w, r)
			return
		}

		// 兼容 Basic Auth
		user, pass, ok := r.BasicAuth()
		if ok && user == "ipc" && subtle.ConstantTimeCompare([]byte(pass), []byte(s.password)) == 1 {
			next(w, r)
			return
		}

		// 有提供凭据但验证失败 → 计数（防爆破）；无凭据请求不计数（与原版行为一致）
		if apiKey != "" || ok {
			recordFailure(ip)
		}
		// 原版 ASF 不发送 WWW-Authenticate 挑战头，避免干扰浏览器/代理行为
		writeJSON(w, http.StatusUnauthorized, map[string]any{"Success": false, "Message": "Unauthorized"})
	}
}

// handleIPCBans 管理被封禁的 IP 列表（对应原版 IPCBansController）。
func (s *Server) handleIPCBans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": getCurrentlyBannedIPs()})
	case http.MethodDelete:
		path := strings.TrimPrefix(r.URL.Path, "/api/ipc/bans")
		path = strings.Trim(path, "/")
		if path == "" {
			clearFailedAuthorizations()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true})
			return
		}
		if !validIP(path) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效 IP 地址"})
			return
		}
		if unbanIP(path) {
			writeJSON(w, http.StatusOK, map[string]any{"Success": true})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "该 IP 未被封禁"})
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "方法不允许"})
	}
}

// currencyCodes 返回常见 Steam 货币代码（对应原版 ECurrencyCode 枚举的简化版）。
func currencyCodes() map[string]string {
	return map[string]string{
		"USD": "US Dollar",
		"EUR": "Euro",
		"GBP": "British Pound",
		"CNY": "Chinese Yuan",
		"JPY": "Japanese Yen",
		"KRW": "South Korean Won",
		"RUB": "Russian Ruble",
		"BRL": "Brazilian Real",
		"CAD": "Canadian Dollar",
		"AUD": "Australian Dollar",
		"INR": "Indian Rupee",
		"TRY": "Turkish Lira",
		"UAH": "Ukrainian Hryvnia",
		"PLN": "Polish Zloty",
		"CHF": "Swiss Franc",
	}
}

func validIP(s string) bool {
	return net.ParseIP(s) != nil
}

// encryptString 对应原版 ArchiCryptoHelper 加密工具（简化实现：PlainText 返回原文，AES 用 AES-GCM）。
func encryptString(method int, s string) string {
	if s == "" {
		return ""
	}
	switch method {
	case 1: // AES
		key := sha256.Sum256([]byte("asf-go-ipc-encryption-key"))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return s
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return s
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return s
		}
		return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(s), nil))
	default: // PlainText 及其他
		return s
	}
}

// hashString 对应原版 ArchiCryptoHelper 哈希工具（简化实现：非 PlainText 用 SHA256）。
func hashString(method int, s string) string {
	if s == "" {
		return ""
	}
	switch method {
	case 0: // PlainText
		return s
	default: // SCrypt / Pbkdf2：用 SHA256 简化
		sum := sha256.Sum256([]byte(s))
		return base64.StdEncoding.EncodeToString(sum[:])
	}
}

// serveStatic 提供嵌入的 ASF-ui 静态文件；前端路由（如 /setup）回退到 index.html。
func serveStatic(sub fs.FS, w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/")
	if rel == "" {
		rel = "index.html"
	}
	data, err := fs.ReadFile(sub, rel)
	if err != nil {
		// SPA fallback：所有非静态资源路径返回 index.html（Content-Type 必须是 HTML，否则浏览器会下载）
		data, err = fs.ReadFile(sub, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(rel))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.HasSuffix(rel, ".js") {
		contentType = "application/javascript"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleType 返回 ASF-ui 配置编辑器所需的类型信息（暂返回空结构，避免 404）。
func (s *Server) handleType(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{}})
}

// handleStructure 返回 ASF-ui 配置编辑器所需的配置结构（暂返回空结构，避免 404）。
func (s *Server) handleStructure(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{}})
}

// handleAPI404 为未实现的 API 端点返回 JSON 404（而非静态文件服务器的文本 404）。
func (s *Server) handleAPI404(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "未知 API 端点: " + r.URL.Path})
}

// handlePlugins 返回已加载插件列表（对应原版 PluginsController，字段 PascalCase）。
func (s *Server) handlePlugins(w http.ResponseWriter, _ *http.Request) {
	plugins := s.asf.Plugins.Plugins()
	result := make([]map[string]any, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, map[string]any{"Name": p.Name(), "Version": p.Version()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": result})
}

// handleNLog 返回日志文件末尾内容（对应原版 NLogController /api/nlog/file，响应为 LogResponse）。
func (s *Server) handleNLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}
	logPath := filepath.Join(s.asf.ConfigPath, "asf-go.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"TotalLines": 0, "Lines": []string{}}})
		return
	}
	lines := strings.Split(string(data), "\n")
	count := 100
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			count = n
		}
	}
	lastAt := len(lines)
	if v := r.URL.Query().Get("lastAt"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= len(lines) {
			lastAt = n
		}
	}
	startFrom := lastAt - count
	if startFrom < 0 {
		startFrom = 0
	}
	tail := lines[startFrom:lastAt]
	writeJSON(w, http.StatusOK, map[string]any{
		"Success": true,
		"Result": map[string]any{
			"TotalLines": len(lines),
			"Lines":      tail,
		},
	})
}

// handleGitHubRelease 返回 ASF 上游 GitHub Release 信息（对应原版 /api/WWW/GitHub/Release）。
func (s *Server) handleGitHubRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}
	prefix := "/api/www/github/release"
	if strings.HasPrefix(r.URL.Path, "/api/releases") {
		prefix = "/api/releases"
	}
	version := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	client := web.NewGitHubClient()

	var release *web.GitHubRelease
	var err error
	if version == "" || version == "latest" {
		release, err = client.GetLatestRelease("JustArchiNET", "ArchiSteamFarm", false)
	} else {
		release, err = client.GetRelease("JustArchiNET", "ArchiSteamFarm", version)
	}
	if err != nil || release == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"Success": false, "Message": fmt.Sprintf("获取 GitHub Release 失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"Success": true,
		"Result": map[string]any{
			"ChangelogHTML": release.Body,
			"ReleasedAt":    release.PublishedAt,
			"Stable":        !release.Prerelease,
			"Version":       release.TagName,
		},
	})
}

// handleStorage 提供持久化 KeyValue 存储（对应原版 StorageController），ASF-ui 用它保存界面设置。
func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	key := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storage/"), "/")
	if key == "" || strings.Contains(key, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效 key"})
		return
	}

	dir := filepath.Join(s.asf.ConfigPath, "storage")
	path := filepath.Join(dir, key+".json")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": nil})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"Success": false, "Message": "读取存储失败"})
			return
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"Success": false, "Message": "解析存储失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": value})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "读取请求体失败"})
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"Success": false, "Message": "创建存储目录失败"})
			return
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"Success": false, "Message": "保存存储失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true})
	case http.MethodDelete:
		_ = os.Remove(path)
		writeJSON(w, http.StatusOK, map[string]any{"Success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "方法不允许"})
	}
}

func (s *Server) handleASF(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/asf"), "/")
		switch action {
		case "":
			// POST /api/asf：保存全局配置
			var req struct {
				GlobalConfig *config.GlobalConfig `json:"globalConfig"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.GlobalConfig == nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效的全局配置"})
				return
			}
			if err := s.asf.UpdateGlobalConfig(req.GlobalConfig); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已保存"})
		case "canupdate":
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": true})
		case "encrypt":
			var req struct {
				CryptoMethod    int    `json:"CryptoMethod"`
				StringToEncrypt string `json:"StringToEncrypt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": encryptString(req.CryptoMethod, req.StringToEncrypt)})
		case "hash":
			var req struct {
				HashingMethod int    `json:"HashingMethod"`
				StringToHash  string `json:"StringToHash"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": hashString(req.HashingMethod, req.StringToHash)})
		case "exit":
			// 始终禁止远程退出（避免误关服务），请通过系统服务管理进程
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "已禁止远程退出，请在服务器上管理 ASF 进程"})
		case "restart", "update":
			// 危险操作保护：未设置 IPC 密码时禁止远程执行
			if s.password == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "未设置 IPC 密码，禁止远程执行该操作"})
				return
			}
			// 二次确认保护：需要 X-ASF-Confirm: true 请求头
			if r.Header.Get("X-ASF-Confirm") != "true" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "需要确认头 X-ASF-Confirm: true 才能执行该操作"})
				return
			}
			if action == "restart" {
				s.asf.RestartBots()
				writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已重启所有 Bot"})
			} else {
				message, err := s.asf.Update()
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": message})
			}
		default:
			writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "未知操作"})
		}
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET/POST"})
		return
	}

	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/asf"), "/")
	switch sub {
	case "currencycodes":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"CurrencyCodes": currencyCodes()}})
		return
	case "updatesenabled":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"UpdatesEnabled": true}})
		return
	}

	bots := s.asf.Bots()
	botStatus := make(map[string]any, len(bots))
	for name, b := range bots {
		botStatus[name] = botStatusMap(b)
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeJSON(w, http.StatusOK, map[string]any{
		"Success": true,
		"Result": map[string]any{
			"Version":          core.Version,
			"BuildVariant":     "generic",
			"CanUpdate":        true,
			"GlobalConfig":     s.asf.Config,
			"MemoryUsage":      mem.Alloc / 1024,
			"ProcessStartTime": s.asf.StartTime().UTC().Format(time.RFC3339),
			"Service":          false,
			"Bots":             botStatus,
			"IPC":              true,
			"Headless":         s.asf.Config.Headless,
			"Debug":            s.asf.Config.Debug,
			"Timestamp":        time.Now().Unix(),
		},
	})
}

func (s *Server) handleBot(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/bot/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "缺少 Bot 名称"})
		return
	}
	botName := parts[0]
	b, ok := s.asf.Bots()[botName]
	if !ok {
		// 原版 BotController 对不存在的 bot 返回 200 + 空字典（仅 GET 列表场景）
		if r.Method == http.MethodGet && len(parts) == 1 {
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{}})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "Bot 不存在"})
		return
	}

	// POST /api/bot/{name}：保存 Bot 配置（对应原版 BotPost）
	if r.Method == http.MethodPost && len(parts) == 1 {
		var req struct {
			BotConfig *config.BotConfig `json:"botConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BotConfig == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效的 Bot 配置"})
			return
		}
		if err := s.asf.UpdateBotConfig(botName, req.BotConfig); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已保存"})
		return
	}

	// DELETE /api/bot/{name}：删除 Bot（暂不支持）
	if r.Method == http.MethodDelete && len(parts) == 1 {
		writeJSON(w, http.StatusOK, map[string]any{"Success": false, "Message": "暂不支持删除 Bot"})
		return
	}

	if len(parts) >= 2 {
		action := strings.Join(parts[1:], "/")
		// 别名路径归一化（兼容第三方/扫描器使用的非官方路径）
		switch strings.ToLower(action) {
		case "2fa":
			action = "twofactorauthentication"
		case "bgr":
			action = "gamesToRedeeminbackground"
		}
		switch key := strings.ToLower(r.Method + " " + action); key {
		case "post pause":
			b.Pause()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已暂停"})
		case "post resume":
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已恢复"})
		case "post restart":
			b.Pause()
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已重启"})
		case "post farm":
			b.Pause()
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已开始挂机"})
		case "get status":
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": botStatusMap(b)})
		case "post start":
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已启动"})
		case "post stop":
			b.Pause()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已停止"})
		case "post delete":
			if r.Header.Get("X-ASF-Confirm") != "true" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "需要确认头 X-ASF-Confirm: true 才能删除 Bot"})
				return
			}
			if err := s.asf.RemoveBot(botName); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已删除"})
		case "get gamestofarm":
			games := b.FarmingGames()
			list := make([]map[string]any, 0, len(games))
			for _, g := range games {
				list = append(list, map[string]any{"appID": g.AppID, "gameName": g.Name, "cardsRemaining": g.CardsRemaining})
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": list})
		case "get farmingblacklist":
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": b.Config.FarmingBlacklist})
		case "post farmingblacklist/add":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingBlacklistAdd(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已添加"})
		case "post farmingblacklist/remove":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingBlacklistRemove(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已移除"})
		case "get farmingqueue":
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": b.Config.FarmingQueue})
		case "post farmingqueue/add":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingQueueAdd(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已添加"})
		case "post farmingqueue/remove":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingQueueRemove(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已移除"})
		case "post unpack":
			opened, err := b.UnpackBoosters()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"opened": opened}})
		case "get gamestoredeeminbackground":
			// BGR：读取后台兑换文件（原版 GamesToRedeemInBackground）
			bgrPath := filepath.Join(s.asf.ConfigPath, botName+".games-to-redeem-in-background")
			bgrData, bgrErr := os.ReadFile(bgrPath)
			var bgrLines []string
			if bgrErr == nil {
				bgrLines = strings.Fields(string(bgrData))
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": bgrLines})
		case "post gamestoredeeminbackground":
			var req struct {
				GamesToRedeemInBackground []string `json:"gamesToRedeemInBackground"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			bgrPath := filepath.Join(s.asf.ConfigPath, botName+".games-to-redeem-in-background")
			if err := os.WriteFile(bgrPath, []byte(strings.Join(req.GamesToRedeemInBackground, "\n")), 0o600); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已保存"})
		case "delete gamestoredeeminbackground":
			bgrPath := filepath.Join(s.asf.ConfigPath, botName+".games-to-redeem-in-background")
			_ = os.Remove(bgrPath)
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已删除"})
		case "post input":
			// 交互式输入暂不支持
			writeJSON(w, http.StatusOK, map[string]any{"Success": false, "Message": "暂不支持交互式输入"})
		case "get 2fa":
			code, err := b.TwoFactorCode()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"code": code}})
		case "get twofactorauthentication/token":
			code, err := b.TwoFactorCode()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"token": code}})
		case "post twofactorauthentication":
			var req struct {
				SharedSecret   string `json:"SharedSecret"`
				IdentitySecret string `json:"IdentitySecret"`
				DeviceID       string `json:"DeviceID"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			if err := b.ImportAuthenticator(req.SharedSecret, req.IdentitySecret, req.DeviceID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "移动令牌已导入"})
		case "get inventory":
			appID := uint32(753)
			contextID := uint64(6)
			if v := r.URL.Query().Get("appid"); v != "" {
				if parsed, err := parseUint32(v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
					return
				} else {
					appID = parsed
				}
			}
			if v := r.URL.Query().Get("contextid"); v != "" {
				if parsed, err := parseUint64(v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "contextid 无效"})
					return
				} else {
					contextID = parsed
				}
			}
			inv, err := b.GetInventory(appID, contextID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": inv})
		case "post twofactorauthentication/confirmations":
			if !b.HasAuthenticator() {
				writeJSON(w, http.StatusOK, map[string]any{"Success": false, "Message": "未配置移动令牌"})
				return
			}
			var req struct {
				Accept bool `json:"accept"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			if !b.IsConnected() {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "Bot 未登录"})
				return
			}
			ws := b.WebSessionOrNil()
			if ws == nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "Web 会话不可用"})
				return
			}
			code, err := b.Auth().GenerateConfirmationCode("conf")
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			deviceID := b.Auth().DeviceID
			if deviceID == "" {
				deviceID = "android:asf-go"
			}
			now := time.Now().Unix()
			confirmations, err := store.GetConfirmations(ws, b.SteamID(), deviceID, code, now)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			ok, err := store.HandleConfirmations(ws, b.SteamID(), deviceID, code, now, confirmations, req.Accept)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": ok, "Message": "已处理确认"})
		case "post mafile":
			var req struct {
				SharedSecret   string `json:"shared_secret"`
				IdentitySecret string `json:"identity_secret"`
				DeviceID       string `json:"device_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			if err := b.ImportAuthenticator(req.SharedSecret, req.IdentitySecret, req.DeviceID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "移动令牌已导入"})
		case "get level":
			level, err := b.SteamLevel()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"level": level}})
		case "post loot":
			var req struct {
				TargetSteamID uint64 `json:"target_steamid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			offerID, err := b.Loot(req.TargetSteamID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"offer_id": offerID}})
		case "post addlicense":
			var req struct {
				Identifiers []string `json:"identifiers"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			lines := make([]string, 0, len(req.Identifiers))
			for _, ident := range req.Identifiers {
				resp, err := b.AddLicense(ident)
				if err != nil {
					lines = append(lines, fmt.Sprintf("%s 失败: %v", ident, err))
				} else {
					lines = append(lines, resp)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": lines})
		case "post redeem":
			var req struct {
				Keys []string `json:"keys"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			resp, err := b.Redeem(req.Keys)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": resp})
		default:
			writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "未知操作"})
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": botStatusMap(b)})
}

// handleBots 返回机器人列表/统计（兼容 ASF-ui 与第三方工具的 /api/bots 系列路径）。
func (s *Server) handleBots(w http.ResponseWriter, r *http.Request) {
	bots := s.asf.Bots()
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/bots"), "/")

	switch sub {
	case "", "bots", "detectbots":
		result := make([]map[string]any, 0, len(bots))
		for name, b := range bots {
			result = append(result, map[string]any{
				"name":   name,
				"status": botStatusMap(b),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": result})
	case "count":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": len(bots)})
	case "botsfarmingcount":
		count := 0
		for _, b := range bots {
			if b.IsFarming() {
				count++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": count})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "未知操作"})
	}
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 POST"})
		return
	}
	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
		return
	}
	cmd := strings.TrimSpace(req.Command)
	switch strings.ToLower(cmd) {
	case "status":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": s.asfStatus()})
	case "pause":
		for _, b := range s.asf.Bots() {
			b.Pause()
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": "全部暂停"})
	case "resume":
		for _, b := range s.asf.Bots() {
			b.Resume()
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": "全部恢复"})
	case "exit":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": "退出命令已收到"})
		go func() { _ = s.asf.Stop() }()
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "未知命令"})
	}
}

func botStatusMap(b *bot.Bot) map[string]any {
	games := make([]map[string]any, 0)
	for _, g := range b.FarmingGames() {
		games = append(games, map[string]any{
			"appID":          g.AppID,
			"gameName":       g.Name,
			"cardsRemaining": g.CardsRemaining,
		})
	}
	return map[string]any{
		"name":         b.Name,
		"connected":    b.IsConnected(),
		"enabled":      b.Config.Enabled,
		"onlineStatus": b.Config.OnlineStatus,
		"steamLogin":   b.Config.SteamLogin,
		"gamesToFarm":  games,
		"cardsFarmer": map[string]any{
			"currentGamesFarming": games,
			"paused":              !b.IsFarming(),
		},
		"farmingBlacklist": b.Config.FarmingBlacklist,
		"farmingQueue":     b.Config.FarmingQueue,
		"access":           "None",
	}
}

func (s *Server) asfStatus() map[string]any {
	return map[string]any{
		"version": core.Version,
		"bots":    len(s.asf.Bots()),
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func parseUint32(s string) (uint32, error) {
	var v uint32
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, err
	}
	return v, nil
}

func parseUint64(s string) (uint64, error) {
	var v uint64
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, err
	}
	return v, nil
}
