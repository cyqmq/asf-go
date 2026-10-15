# Changelog

所有重要变更都记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [0.5.0] - 2026-10-15

### 新增
- IPC 认证支持 `IPCPasswordFormat`（PlainText / PBKDF2 / SCrypt）哈希验证
- CORS 来源白名单（`GlobalConfig.IPCOrigins`）
- Steam 密码与登录 Key 支持 `ASF_CRYPTKEY` 环境变量加密存储
- 聊天命令：`!2fa`、`!mafile`、`!password`、`!keys`、`!batch`
- SteamTradeMatcher 公平交易匹配（`TradingPreferencesSteamTradeMatcher` + `MatchableTypes`）
- ASF-ui 配置编辑器所需端点：`/api/type/{type}`、`/api/structure/{type}`、GitHub wiki 代理
- ASF-ui 硬编码 GitHub 链接改为本项目仓库
- CI：`go vet` + `go test -race` + `golangci-lint`
- 模糊测试（游戏标识符解析、聊天命令、IPC 路径解析）
- IPC 集成测试与配置兼容性测试

### 工程结构
- 拆分 `internal/bot/bot.go` 与 `internal/ipc/server.go` 为按域文件
- 新增统一错误模型（`APIError` + 错误码）

### 稳定性
- 断线自动重连（指数退避）
- QR 登录字段并发安全
- 挂机游戏列表轻量状态持久化

## [0.2.1] - 2026-10-14

### 新增
- GitHub Actions 多平台发布（linux/darwin/windows amd64+arm64）
- 动态插件加载（`.so`）
- 自动确认交易/市场挂单

## [0.2.0] - 2026-10-14

### 新增
- 完整 IPC 端点与官方 ASF-ui 集成
- 交易、库存、权限、更新检查模块
- 聊天命令体系

## [0.1.0-go] - 2026-10-12

### 新增
- 初始 Go 重写：配置加载、Steam 登录、卡牌挂机、基础命令