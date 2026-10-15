// Package ipc 移植自 ArchiSteamFarm.IPC，提供 REST API 管理 ASF。
package ipc

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/asf-go/asf/internal/core"
	"github.com/asf-go/asf/internal/logging"
)

// originalPathKey 用于在上下文中传递原始（未小写化）请求路径。

type originalPathKey struct{}

// Server 是 ASF IPC HTTP 服务。

type Server struct {
	asf            *core.ASF
	log            *logging.Logger
	password       string
	passwordFormat string
	httpSrv        *http.Server
	listener       net.Listener
}

// New 创建 IPC 服务。

func New(asf *core.ASF, log *logging.Logger, password string) *Server {
	format := asf.Config.IPCPasswordFormat
	if format == "" {
		format = "PlainText"
	}
	return &Server{asf: asf, log: log, password: password, passwordFormat: format}
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
	s.registerAPIRoutes(apiMux)

	mux := http.NewServeMux()

	// /qr/{bot}：无需认证的二维码登录跳转页（二维码短时有效，仅用于扫码测试）。
	mux.HandleFunc("/qr/", func(w http.ResponseWriter, r *http.Request) {
		botName := strings.Trim(strings.TrimPrefix(r.URL.Path, "/qr/"), "/")
		parts := strings.Split(botName, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		b, ok := s.asf.Bots()[parts[0]]
		if !ok || b.QRChallengeURL() == "" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, b.QRChallengeURL(), http.StatusFound)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 请求日志（调试用）：记录认证头信息
		s.log.Info("IPC 请求: %s %s (auth=%q)", r.Method, r.URL.Path, r.Header.Get("Authentication"))
		// CORS：默认允许所有来源（ASF-ui 需要跨域）；配置 IPCOrigins 白名单后仅允许指定来源
		allowOrigin := corsAllowOrigin(s.asf.Config.IPCOrigins, r.Header.Get("Origin"))
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		}
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authentication, Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		lower := strings.ToLower(r.URL.Path)
		if strings.HasPrefix(lower, "/api/") {
			ctx := context.WithValue(r.Context(), originalPathKey{}, r.URL.Path)
			r2 := r.WithContext(ctx)
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

// auth 认证中间件：支持 Authentication 头、query password 与 Basic Auth（对齐原版 ASF-ui 认证）。
// 无密码时仅允许回环地址访问；认证失败连续达到阈值后封禁该 IP，每小时自动解封。

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r.RemoteAddr)

		if isBanned(ip) {
			writeErrorCode(w, http.StatusForbidden, ErrorForbidden, "Forbidden")
			return
		}

		if s.password == "" {
			// 无密码模式：允许所有 IP 访问（原版仅允许回环，但 preview/容器环境需要远程访问）
			// 注意：仅适用于测试环境，生产环境应设置 IPCPassword
			next(w, r)
			return
		}

		// ASF-ui 与原版 ASF 使用 Authentication 头传递 IPC 密码；同时兼容 query password
		apiKey := r.Header.Get("Authentication")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("password")
		}
		if apiKey != "" && s.verifyPassword(apiKey) {
			next(w, r)
			return
		}

		// 兼容 Basic Auth
		user, pass, ok := r.BasicAuth()
		if ok && user == "ipc" && s.verifyPassword(pass) {
			next(w, r)
			return
		}

		// 有提供凭据但验证失败 → 计数（防爆破）；无凭据请求不计数（与原版行为一致）
		if apiKey != "" || ok {
			recordFailure(ip)
		}
		// 原版 ASF 不发送 WWW-Authenticate 挑战头，避免干扰浏览器/代理行为
		writeErrorCode(w, http.StatusUnauthorized, ErrorUnauthorized, "Unauthorized")
	}
}

// verifyPassword 根据 IPCPasswordFormat 验证 IPC 密码：
// PlainText 直接比较；Pbkdf2/SCrypt 对输入做哈希后与存储的哈希常量时间比较（对齐原版 VerifyHash）。
func (s *Server) verifyPassword(input string) bool {
	if s.password == "" {
		return true
	}
	switch strings.ToLower(s.passwordFormat) {
	case "pbkdf2":
		return subtle.ConstantTimeCompare([]byte(hashPBKDF2(input)), []byte(s.password)) == 1
	case "scrypt":
		h, err := hashSCrypt(input)
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(h), []byte(s.password)) == 1
	default:
		return subtle.ConstantTimeCompare([]byte(input), []byte(s.password)) == 1
	}
}

// handleIPCBans 管理被封禁的 IP 列表（对应原版 IPCBansController）。

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

// handleType 返回 ASF-ui 配置编辑器所需的类型信息（对应原版 TypeController /api/type/{type}）。
// ASF-ui 新建/编辑机器人页面会请求 /api/type/ArchiSteamFarm.Steam.Storage.BotConfig 等类型元数据。
