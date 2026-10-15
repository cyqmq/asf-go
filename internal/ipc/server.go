// Package ipc 移植自 ArchiSteamFarm.IPC，提供 REST API 管理 ASF。
package ipc

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/bot"
	"github.com/asf-go/asf/internal/core"
	"github.com/asf-go/asf/internal/logging"
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
	fileServer := http.FileServer(http.FS(sub))

	// API 路由（统一小写匹配，兼容原版 ASP.NET 大小写不敏感路由）
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/asf", s.auth(s.handleASF))
	apiMux.HandleFunc("/api/bot/", s.auth(s.handleBot))
	apiMux.HandleFunc("/api/commands", s.auth(s.handleCommands))
	apiMux.HandleFunc("/api/ipc/bans", s.auth(s.handleIPCBans))
	apiMux.HandleFunc("/api/type", s.auth(s.handleType))
	apiMux.HandleFunc("/api/structure", s.auth(s.handleStructure))
	apiMux.HandleFunc("/api/storage/", s.auth(s.handleStorage))
	apiMux.HandleFunc("/api/", s.auth(s.handleAPI404))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 请求日志（调试用）
		s.log.Info("IPC 请求: %s %s", r.Method, r.URL.Path)
		// CORS：ASF-ui 携带 X-APIKey 自定义头，需要处理预检
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "X-APIKey, Content-Type, Authorization")
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
		fileServer.ServeHTTP(w, r)
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
			writeJSON(w, http.StatusForbidden, map[string]any{"success": false, "message": "Forbidden"})
			return
		}

		if s.password == "" {
			parsed := net.ParseIP(ip)
			if parsed == nil || !parsed.IsLoopback() {
				writeJSON(w, http.StatusForbidden, map[string]any{"success": false, "message": "Forbidden"})
				return
			}
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
		w.Header().Set("WWW-Authenticate", `Basic realm="ASF"`)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "Unauthorized"})
	}
}

// handleIPCBans 管理被封禁的 IP 列表（对应原版 IPCBansController）。
func (s *Server) handleIPCBans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": getCurrentlyBannedIPs()})
	case http.MethodDelete:
		path := strings.TrimPrefix(r.URL.Path, "/api/ipc/bans")
		path = strings.Trim(path, "/")
		if path == "" {
			clearFailedAuthorizations()
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
			return
		}
		if !validIP(path) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "无效 IP 地址"})
			return
		}
		if unbanIP(path) {
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "该 IP 未被封禁"})
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "方法不允许"})
	}
}

func validIP(s string) bool {
	return net.ParseIP(s) != nil
}

// handleType 返回 ASF-ui 配置编辑器所需的类型信息（暂返回空结构，避免 404）。
func (s *Server) handleType(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{}})
}

// handleStructure 返回 ASF-ui 配置编辑器所需的配置结构（暂返回空结构，避免 404）。
func (s *Server) handleStructure(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{}})
}

// handleAPI404 为未实现的 API 端点返回 JSON 404（而非静态文件服务器的文本 404）。
func (s *Server) handleAPI404(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "message": "未知 API 端点: " + r.URL.Path})
}

// handleStorage 提供持久化 KeyValue 存储（对应原版 StorageController），ASF-ui 用它保存界面设置。
func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	key := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storage/"), "/")
	if key == "" || strings.Contains(key, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "无效 key"})
		return
	}

	dir := filepath.Join(s.asf.ConfigPath, "storage")
	path := filepath.Join(dir, key+".json")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": nil})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "读取存储失败"})
			return
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "解析存储失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": value})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "读取请求体失败"})
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "创建存储目录失败"})
			return
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": "保存存储失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	case http.MethodDelete:
		_ = os.Remove(path)
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "方法不允许"})
	}
}

func (s *Server) handleASF(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		switch strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/asf"), "/") {
		case "Exit":
			s.asf.Exit()
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "ASF 正在退出"})
		case "Restart":
			s.asf.RestartBots()
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已重启所有 Bot"})
		case "Update":
			message, err := s.asf.Update()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": message})
		default:
			writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "message": "未知操作"})
		}
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "仅支持 GET/POST"})
		return
	}

	bots := s.asf.Bots()
	botStatus := make(map[string]any, len(bots))
	for name, b := range bots {
		botStatus[name] = botStatusMap(b)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result": map[string]any{
			"version":   core.Version,
			"bots":      botStatus,
			"ipc":       true,
			"headless":  s.asf.Config.Headless,
			"debug":     s.asf.Config.Debug,
			"timestamp": time.Now().Unix(),
		},
	})
}

func (s *Server) handleBot(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/bot/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "缺少 Bot 名称"})
		return
	}
	botName := parts[0]
	b, ok := s.asf.Bots()[botName]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "message": "Bot 不存在"})
		return
	}

	if len(parts) >= 2 {
		action := parts[1]
		switch r.Method + " " + action {
		case "POST pause":
			b.Pause()
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已暂停"})
		case "POST resume":
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已恢复"})
		case "POST restart":
			b.Pause()
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已重启"})
		case "POST farm":
			b.Pause()
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已开始挂机"})
		case "GET gamesToFarm":
			games := b.FarmingGames()
			list := make([]map[string]any, 0, len(games))
			for _, g := range games {
				list = append(list, map[string]any{"appID": g.AppID, "gameName": g.Name, "cardsRemaining": g.CardsRemaining})
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": list})
		case "GET farmingBlacklist":
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": b.Config.FarmingBlacklist})
		case "POST farmingBlacklist/add":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "appid 无效"})
				return
			}
			if err := b.FarmingBlacklistAdd(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已添加"})
		case "POST farmingBlacklist/remove":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "appid 无效"})
				return
			}
			if err := b.FarmingBlacklistRemove(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已移除"})
		case "GET farmingQueue":
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": b.Config.FarmingQueue})
		case "POST farmingQueue/add":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "appid 无效"})
				return
			}
			if err := b.FarmingQueueAdd(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已添加"})
		case "POST farmingQueue/remove":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "appid 无效"})
				return
			}
			if err := b.FarmingQueueRemove(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已移除"})
		case "POST unpack":
			opened, err := b.UnpackBoosters()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{"opened": opened}})
		case "GET gamesToRedeemInBackground":
			// BGR（后台兑换）尚未实现，返回空结果以兼容 ASF-ui
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{}})
		case "POST input":
			// 交互式输入暂不支持
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "暂不支持交互式输入"})
		case "GET 2fa":
			code, err := b.TwoFactorCode()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{"code": code}})
		case "GET inventory":
			appID := uint32(753)
			contextID := uint64(6)
			if v := r.URL.Query().Get("appid"); v != "" {
				if parsed, err := parseUint32(v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "appid 无效"})
					return
				} else {
					appID = parsed
				}
			}
			if v := r.URL.Query().Get("contextid"); v != "" {
				if parsed, err := parseUint64(v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "contextid 无效"})
					return
				} else {
					contextID = parsed
				}
			}
			inv, err := b.GetInventory(appID, contextID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": inv})
		case "POST mafile":
			var req struct {
				SharedSecret   string `json:"shared_secret"`
				IdentitySecret string `json:"identity_secret"`
				DeviceID       string `json:"device_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			if err := b.ImportAuthenticator(req.SharedSecret, req.IdentitySecret, req.DeviceID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "移动令牌已导入"})
		case "GET level":
			level, err := b.SteamLevel()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{"level": level}})
		case "POST loot":
			var req struct {
				TargetSteamID uint64 `json:"target_steamid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			offerID, err := b.Loot(req.TargetSteamID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{"offer_id": offerID}})
		case "POST addlicense":
			var req struct {
				Identifiers []string `json:"identifiers"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
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
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": lines})
		case "POST redeem":
			var req struct {
				Keys []string `json:"keys"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			resp, err := b.Redeem(req.Keys)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": resp})
		default:
			writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "message": "未知操作"})
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": botStatusMap(b)})
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "仅支持 POST"})
		return
	}
	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
		return
	}
	cmd := strings.TrimSpace(req.Command)
	switch strings.ToLower(cmd) {
	case "status":
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": s.asfStatus()})
	case "pause":
		for _, b := range s.asf.Bots() {
			b.Pause()
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": "全部暂停"})
	case "resume":
		for _, b := range s.asf.Bots() {
			b.Resume()
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": "全部恢复"})
	case "exit":
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": "退出命令已收到"})
		go func() { _ = s.asf.Stop() }()
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "未知命令"})
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
