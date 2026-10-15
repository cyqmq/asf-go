package ipc

import "net/http"

// registerAPIRoutes 注册全部 API 路由。
// 路径统一小写匹配（兼容原版 ASP.NET 大小写不敏感路由），Bot 名等原始大小写通过 context 保留。
func (s *Server) registerAPIRoutes(apiMux *http.ServeMux) {
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
	apiMux.HandleFunc("/api/www/github/wiki/page/", s.auth(s.handleGitHubWiki))
	apiMux.HandleFunc("/api/www/github/wiki/history/", s.auth(s.handleGitHubWiki))
	apiMux.HandleFunc("/api/releases", s.auth(s.handleGitHubRelease))
	apiMux.HandleFunc("/api/releases/", s.auth(s.handleGitHubRelease))
	apiMux.HandleFunc("/api/type", s.auth(s.handleType))
	apiMux.HandleFunc("/api/type/", s.auth(s.handleType))
	apiMux.HandleFunc("/api/structure", s.auth(s.handleStructure))
	apiMux.HandleFunc("/api/structure/", s.auth(s.handleStructure))
	apiMux.HandleFunc("/api/storage/", s.auth(s.handleStorage))
	apiMux.HandleFunc("/api/plugins", s.auth(s.handlePlugins))
	apiMux.HandleFunc("/api/", s.auth(s.handleAPI404))
}
