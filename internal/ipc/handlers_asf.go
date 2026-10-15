package ipc

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/core"
)

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
			if req.StringToEncrypt == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "StringToEncrypt 不能为空"})
				return
			}
			switch req.CryptoMethod {
			case 2: // ProtectedDataForCurrentUser：仅 Windows 支持
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "当前平台不支持 ProtectedDataForCurrentUser"})
				return
			case 3: // EnvironmentVariable：返回环境变量值
				writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": os.Getenv(req.StringToEncrypt)})
				return
			case 4: // File：出于安全考虑不支持读取任意文件
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "暂不支持 File 加密方式"})
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
			if req.StringToHash == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "StringToHash 不能为空"})
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET/POST"})
		return
	}

	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/asf"), "/")
	switch strings.ToLower(sub) {
	case "version":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": core.Version})
		return
	case "buildvariant":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": "generic"})
		return
	case "currencycodes":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"CurrencyCodes": currencyCodes()}})
		return
	case "updatesenabled":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"UpdatesEnabled": true}})
		return
	case "canupdate":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": true})
		return
	case "memory":
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": memStats.Alloc / 1024})
		return
	case "uptime":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": time.Since(s.asf.StartTime()).String()})
		return
	case "hash", "encrypt", "confirmations":
		// 原版这些端点为 POST 语义，GET 应返回 405
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "请使用 POST 方法"})
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
