# ASF-Go：ArchiSteamFarm 的 Go 语言重写

将 [ArchiSteamFarm](https://github.com/JustArchiNET/ArchiSteamFarm)（C#/.NET）用 Go 重写的移植项目。已实现核心链路：**配置加载 → Bot 启动 → Steam 登录 → 卡牌挂机 → 聊天命令 → IPC Web API**。

## 架构对应关系

| ASF 原 C# 模块 | Go 移植包 |
|----------------|-----------|
| `ArchiSteamFarm.Storage.GlobalConfig` | `internal/config`（`GlobalConfig`） |
| `ArchiSteamFarm.Steam.Storage.BotConfig` | `internal/config`（`BotConfig`） |
| `ArchiSteamFarm.NLog` | `internal/logging` |
| `ArchiSteamFarm.Core.ASF` | `internal/core` |
| `ArchiSteamFarm.Steam.Bot` | `internal/bot` |
| `ArchiSteamFarm.Steam.Cards.CardsFarmer` | `internal/bot`（`CardsFarmer`） |
| `ArchiSteamFarm.Steam.SteamKit2` | `internal/steam`（基于 `github.com/Philipp15b/go-steam/v3`） |
| `ArchiSteamFarm.IPC` | `internal/ipc`（Go `net/http`） |
| `ArchiSteamFarm.Web.GitHub` | `internal/web`（GitHub 更新检查） |
| `ArchiSteamFarm.Localization` | `internal/localization`（中/英文本） |
| `ArchiSteamFarm.Plugins` | `internal/plugins`（插件接口与管理器） |

## 目录结构

```
asf-go/
├── cmd/asf/main.go            # 入口：--config <配置目录> --ipc-addr <监听地址>
└── internal/
    ├── config/                # GlobalConfig / BotConfig（JSON PascalCase，兼容 ASF）
    ├── logging/               # 分级日志（控制台 + 文件）
    ├── core/                  # ASF 核心：配置加载、Bot 发现与生命周期
    ├── bot/                   # Bot：Steam 登录、登录 Key 持久化、命令处理
    ├── bot/cards_farmer.go    # 卡牌挂机：解析 badges 页面 -> 设置游玩游戏
    ├── steam/                 # Steam 协议客户端封装 + Web 会话
    └── ipc/                   # REST API（ASF/Bot/Commands）
```

## 使用方法

```bash
# 构建
go build -o asf ./cmd/asf

# 运行（config 目录需包含 ASF.json 和 <BotName>.json）
./asf --config /path/to/config

# 可选：IPC 监听地址（默认 :1242）
./asf --config /path/to/config --ipc-addr 127.0.0.1:1242
```

### 配置格式（与 ASF 兼容）

`ASF.json`：
```json
{
  "AutoRestart": true,
  "CommandPrefix": "!",
  "SteamOwnerID": 76561198000000000,
  "IPC": true,
  "IPCPassword": "your_password"
}
```

`testbot.json`：
```json
{
  "Enabled": true,
  "SteamLogin": "your_login",
  "SteamPassword": "your_password",
  "OnlineStatus": "Online"
}
```

### Steam Guard

通过环境变量注入验证码（登录时使用）：
```bash
STEAM_AUTH=abc123 STEAM_2FA=123456 ./asf --config /path/to/config
```

## IPC API

| 端点 | 方法 | 说明 |
|-----|------|------|
| `/api/ASF` | GET | ASF 状态、版本、Bot 列表 |
| `/api/bot/{name}` | GET | Bot 状态（连接、挂机游戏） |
| `/api/bot/{name}/pause` | POST | 暂停挂机 |
| `/api/bot/{name}/resume` | POST | 恢复挂机 |
| `/api/bot/{name}/inventory` | GET | 查询库存（`appid`、`contextid` 参数，默认 753/6） |
| `/api/bot/{name}/2fa` | GET | 生成 2FA 验证码 |
| `/api/bot/{name}/mafile` | POST | 导入移动令牌（shared_secret / identity_secret） |
| `/api/bot/{name}/level` | GET | 查询 Steam 等级 |
| `/api/bot/{name}/loot` | POST | 发送战利品（target_steamid） |
| `/api/bot/{name}/addlicense` | POST | 激活免费许可（identifiers 数组） |
| `/api/bot/{name}/redeem` | POST | 兑换激活码（keys 数组） |
| `/api/commands` | POST | 执行命令（status/pause/resume/exit） |

认证：`IPCPassword` 为空时无需认证；非空时使用 HTTP Basic Auth（用户名 `ipc`）。

## 已实现

- [x] GlobalConfig / BotConfig JSON 加载与默认值（兼容 ASF 配置）
- [x] 分级日志（控制台 + 文件）
- [x] ASF 核心初始化、Bot 自动发现（扫描 `*.json`）、后台启动
- [x] Bot Steam 登录（go-steam 协议库 `github.com/Philipp15b/go-steam/v3`）+ 登录 Key 持久化
- [x] Steam Guard 邮箱验证码 / 2FA 手机令牌（环境变量注入）
- [x] 2FA 移动令牌生成（对应 `MobileAuthenticator` 插件，TOTP 基于 `github.com/Philipp15b/go-steam/v3/totp`）
- [x] Steam Web API 徽章查询（`SteamWebApiKey` 配置项，基于 `github.com/gofurry/steam-go`）
- [x] Web 会话初始化（steamcommunity Cookie）
- [x] CardsFarmer 卡牌挂机（解析 badges 页面 `badge_row_inner`，设置 `GC.SetGamesPlayed`）
- [x] 库存查询（基于 `github.com/Philipp15b/go-steam/v3/economy/inventory`，IPC 端点）
- [x] 交易报价处理（IEconService Web API，支持 AcceptAll / 礼物 / SteamlvlLimit / 黑名单 / 允许列表）
- [x] 发送交易报价（`!loot <SteamID64>`，基于 v3 `tradeoffer` 子包，自动收集卡片/闪卡/补充包发送）
- [x] 定时自动发送战利品（`SendTradePeriod`，按周期把可转移物品发给允许列表 / Owner）
- [x] 免费许可激活（`!addlicense sub/<subid>`，复刻原版 ArchiWebHandler.AddFreeLicense）
- [x] 免费许可激活（`!addlicense app/<appid>`，通过自维护 go-steam 的 SteamApps handler 走 CM 协议）
- [x] Beta 分支检查（`!checkappbeta <appid> <password>`，基于自维护 go-steam 的 SteamApps handler）
- [x] 游戏所有权查询（`!owns <appid>`，基于 steam-go Web API 的 GetOwnedGames）
- [x] 激活码兑换（`!redeem <key>`，Web 方式等价实现原版 RedeemKey）
- [x] 权限体系（`SteamUserPermissions` + Owner，按原版 EAccess 控制各命令）
- [x] 扩展命令集（`!farm` / `!level` / `!stats` / `!version` / `!exit`）
- [x] 游戏标识符解析（`app/`、`sub/`、商店链接、steam:// 链接，复刻原版 TryParseGameIdentifier）
- [x] 好友请求处理（`AcceptFriendRequests` 自动接受 / `BotBehaviour.RejectInvalidFriendRequests` 拒绝无效请求）
- [x] 聊天/群组邀请处理（`BotBehaviour.RejectInvalidGroupInvites`，可自动加入）
- [x] 聊天命令处理（`!status` / `!pause` / `!resume` / `!help`）
- [x] IPC REST API（ASF / Bot / Commands，Basic Auth）
- [x] GitHub 更新检查（上游版本对比）
- [x] 本地化（英文/中文文本表）
- [x] 插件系统（Plugin 接口 + 管理器 + 内置示例插件）
- [x] 多 Bot 支持（扫描配置目录所有 `<BotName>.json`）
- [x] 单元测试（config、CardsFarmer 解析、IPC、插件、GitHub 版本比较、authenticator、trading、好友策略、Loot 筛选）

## 待实现（Roadmap）

- [ ] 动态插件加载（Go `plugin` 包，对应 ASF 的插件热加载）
- [ ] 库存批量转移 / 兑换（`Inventory`、`Exchange` 模块）
- [ ] 群组/好友通知（主动向 Owner 推送交易、好友事件）

## 许可证

遵循原项目 [Apache-2.0](LICENSE) 许可（本项目为独立重写实现）。