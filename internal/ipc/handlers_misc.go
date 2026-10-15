package ipc

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
// encryptString 对应原版 Actions.Encrypt。
// CryptoMethod 枚举：0=PlainText 1=AES 2=ProtectedDataForCurrentUser 3=EnvironmentVariable 4=File。

func encryptString(method int, s string) string {
	switch method {
	case 1: // AES
		out, err := encryptAES(s)
		if err != nil {
			return s
		}
		return out
	case 3: // EnvironmentVariable：原版返回环境变量值（不转换）
		return s
	case 4: // File：原版读取文件内容（出于安全考虑本实现不读取任意文件）
		return s
	default: // PlainText 及其他
		return s
	}
}

// hashString 对应原版 Actions.Hash。
// HashingMethod 枚举：0=PlainText 1=SCrypt 2=Pbkdf2。

func hashString(method int, s string) string {
	switch method {
	case 0: // PlainText
		return s
	case 1: // SCrypt
		out, err := hashSCrypt(s)
		if err != nil {
			return ""
		}
		return out
	case 2: // Pbkdf2
		return hashPBKDF2(s)
	default:
		return s
	}
}

// serveStatic 提供嵌入的 ASF-ui 静态文件；前端路由（如 /setup）回退到 index.html。

func (s *Server) handleAPI404(w http.ResponseWriter, r *http.Request) {
	writeErrorCode(w, http.StatusNotFound, ErrorNotFound, "未知 API 端点: "+r.URL.Path)
}

// handlePlugins 返回已加载插件列表（对应原版 PluginsController，字段 PascalCase）。

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}
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
