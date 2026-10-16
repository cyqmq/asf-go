// Package bot 移植自 ArchiSteamFarm.Steam.Bot（C# 核心 Bot 逻辑的精简版）。
package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asf-go/asf/internal/authenticator"
	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/logging"
	"github.com/asf-go/asf/internal/steam"
)

// Bot 对应 ASF 的 Bot 对象，表示一个独立的 Steam 账号机器人。

type Bot struct {
	Name   string
	Config *config.BotConfig
	Log    *logging.Logger

	CommandPrefix  string
	SteamAPIKey    string
	OwnerSteamID   uint64
	ASFVersion     string
	OnExit         func()
	Blacklist      []uint64
	FindBotSteamID func(name string) uint64
	GlobalConfig   *config.GlobalConfig

	configDir   string
	loginKey    string
	steamClient *steam.Client
	cardsFarmer *CardsFarmer
	auth        *authenticator.Authenticator

	reconnecting atomic.Bool

	tradingStopCh       chan struct{}
	tradingDoneCh       chan struct{}
	confirmationsStopCh chan struct{}

	// QR 扫码登录相关
	qrMu           sync.Mutex
	qrSession      *steam.QRLoginSession
	qrChallengeURL string
	qrStopCh       chan struct{}

	// 主动匹配黑名单（对应原版 BotDatabase.MatchActivelyBlacklistAppIDs）
	mabMutex  sync.RWMutex
	mabAppIDs []uint32
}

// New 创建 Bot 实例。configDir 用于持久化登录 Key。

func New(name string, cfg *config.BotConfig, configDir string) (*Bot, error) {
	log, err := logging.New("Bot-"+name, filepath.Join(configDir, "asf-go.log"), logging.Info)
	if err != nil {
		return nil, err
	}
	b := &Bot{
		Name:      name,
		Config:    cfg,
		Log:       log,
		configDir: configDir,
	}
	b.loginKey = b.loadLoginKey()
	b.auth = b.loadAuthenticator()
	b.loadMAB()
	return b, nil
}

// newSteamClient 创建 Steam 客户端，优先使用 BotConfig 代理，否则回退到 GlobalConfig 代理。
func (b *Bot) newSteamClient(username, password, loginKey string) *steam.Client {
	proxyText := b.Config.WebProxyText
	proxyUsername := b.Config.WebProxyUsername
	proxyPassword := b.Config.GetDecryptedWebProxyPassword()
	if proxyText == "" && b.GlobalConfig != nil {
		proxyText = b.GlobalConfig.WebProxyText
		proxyUsername = b.GlobalConfig.WebProxyUsername
		proxyPassword = b.GlobalConfig.GetDecryptedWebProxyPassword()
	}
	b.checkSteamProtocols()
	return steam.NewClientWithProxy(b.Log, username, password, loginKey, proxyText, proxyUsername, proxyPassword)
}

// checkSteamProtocols 校验 SteamProtocols 配置。go-steam 仅支持 TCP 连接，
// 当用户指定 WebSocket/UDP 时记录警告（原版支持协议选择，本构建受限）。
func (b *Bot) checkSteamProtocols() {
	if b.GlobalConfig == nil {
		return
	}
	protocols := strings.ToLower(strings.TrimSpace(b.GlobalConfig.SteamProtocols))
	if protocols == "websocket" || protocols == "udp" || protocols == "web" {
		b.Log.Warn("SteamProtocols=%s 但当前构建仅支持 TCP 连接，已忽略", b.GlobalConfig.SteamProtocols)
	}
}

// Start 连接 Steam 并登录。

func (b *Bot) Start() error {
	b.Log.Info("正在启动 Bot %s", b.Name)
	if !b.Config.Enabled {
		b.Log.Info("Bot 未启用，跳过登录")
		return nil
	}
	if b.Config.SteamLogin == "" {
		// 未配置账号名：使用 QR 扫码登录（对应原版 WantsQrCodeLogin）。
		return b.startQRLogin()
	}

	client := b.newSteamClient(b.Config.SteamLogin, b.Config.GetDecryptedSteamPassword(), b.loginKey)
	client.SetOnLoginKey(func(key string) {
		b.loginKey = key
		b.saveLoginKey()
	})
	client.SetOnChatMessage(b.handleCommand)
	client.SetOnFriendRequest(b.handleFriendRequest)
	client.SetOnChatInvite(b.handleChatInvite)
	client.SetOnDisconnect(func() { b.scheduleReconnect() })
	// Steam Guard：优先使用 maFile 生成令牌码；否则支持环境变量注入
	if b.auth != nil {
		if code, err := b.auth.GenerateCode(); err == nil {
			client.SetTwoFactorCode(code)
			b.Log.Info("已使用移动令牌生成 2FA 验证码")
		} else {
			b.Log.Warn("生成 2FA 验证码失败: %v", err)
		}
	}
	if code := os.Getenv("STEAM_AUTH"); code != "" {
		client.SetAuthCode(code)
	}
	if code := os.Getenv("STEAM_2FA"); code != "" {
		client.SetTwoFactorCode(code)
	}
	b.steamClient = client

	if err := client.Connect(90 * time.Second); err != nil {
		return fmt.Errorf("Bot %s 连接失败: %w", b.Name, err)
	}

	b.finishStartup()
	b.Log.Info("Bot %s 已就绪", b.Name)
	return nil
}

// startQRLogin 启动 QR 扫码登录：创建 Web QR 会话并暴露 challenge URL，
// 后台轮询扫码状态，确认后自动连接 CM 并登录。

func (b *Bot) startQRLogin() error {
	client := b.newSteamClient("", "", "")
	client.SetOnLoginKey(func(key string) {
		b.loginKey = key
		b.saveLoginKey()
	})
	client.SetOnChatMessage(b.handleCommand)
	client.SetOnFriendRequest(b.handleFriendRequest)
	client.SetOnChatInvite(b.handleChatInvite)
	client.SetOnDisconnect(func() { b.scheduleReconnect() })
	b.steamClient = client

	session, challengeURL, err := steam.StartQRLogin(nil)
	if err != nil {
		b.Log.Error("启动 QR 登录失败: %v", err)
		return fmt.Errorf("Bot %s 启动 QR 登录失败: %w", b.Name, err)
	}
	b.qrMu.Lock()
	b.qrSession = session
	b.qrChallengeURL = challengeURL
	b.qrStopCh = make(chan struct{})
	b.qrMu.Unlock()
	b.Log.Info("QR 登录已启动，等待扫码: %s", challengeURL)

	go b.pollQRLogin()
	return nil
}

// pollQRLogin 轮询 QR 扫码状态，完成后用令牌登录。

func (b *Bot) pollQRLogin() {
	interval := time.Duration(b.qrSession.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	stopCh := b.qrStopCh
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			done, err := b.qrSession.Poll()
			if err != nil {
				b.Log.Debug("QR 轮询失败: %v", err)
				continue
			}
			// 同步 Steam 刷新的挑战 URL（二维码有效期约 60 秒）
			b.qrMu.Lock()
			b.qrChallengeURL = b.qrSession.ChallengeURL
			b.qrMu.Unlock()
			if !done {
				continue
			}
			b.Log.Info("QR 扫码已确认，账号 %s，开始登录", b.qrSession.AccountName)
			b.steamClient.SetUsername(b.qrSession.AccountName)
			b.steamClient.SetAccessToken(b.qrSession.LoginToken())
			b.qrMu.Lock()
			b.qrChallengeURL = ""
			b.qrMu.Unlock()
			if err := b.steamClient.Connect(90 * time.Second); err != nil {
				b.Log.Error("Bot %s 连接失败: %v", b.Name, err)
				return
			}
			b.finishStartup()
			b.Log.Info("Bot %s 已就绪", b.Name)
			return
		}
	}
}

// finishStartup 在登录成功后初始化 Web 会话、挂卡、交易与确认模块。

func (b *Bot) finishStartup() {
	// 初始化 Web 会话（卡牌挂机需要）
	if err := b.steamClient.InitWebSession(); err != nil {
		b.Log.Warn("Web 会话初始化失败: %v", err)
	}

	// 启动卡牌挂机
	b.cardsFarmer = NewCardsFarmer(b)
	b.cardsFarmer.Start()

	// 启动交易报价处理（若启用）
	b.startTrading()

	// 启动确认自动处理（若已导入移动令牌）
	b.startConfirmations()
}

// QRChallengeURL 返回当前 QR 登录挑战 URL（未启动 QR 登录时为空）。

func (b *Bot) QRChallengeURL() string {
	b.qrMu.Lock()
	defer b.qrMu.Unlock()
	return b.qrChallengeURL
}

// CancelQRLogin 取消正在进行的 QR 登录轮询。

func (b *Bot) CancelQRLogin() {
	b.qrMu.Lock()
	defer b.qrMu.Unlock()
	if b.qrStopCh != nil {
		close(b.qrStopCh)
		b.qrStopCh = nil
	}
	b.qrChallengeURL = ""
}

// Stop 断开 Steam 连接。

func (b *Bot) Stop() error {
	b.CancelQRLogin()
	b.stopTrading()
	if b.confirmationsStopCh != nil {
		close(b.confirmationsStopCh)
		b.confirmationsStopCh = nil
	}
	if b.cardsFarmer != nil {
		b.cardsFarmer.Stop()
	}
	if b.steamClient != nil {
		b.steamClient.Disconnect()
	}
	b.Log.Info("Bot %s 已停止", b.Name)
	return b.Log.Close()
}

// stopModules 停止所有运行中的后台模块（不关闭日志），用于断线重连前清理。
func (b *Bot) stopModules() {
	b.CancelQRLogin()
	b.stopTrading()
	if b.confirmationsStopCh != nil {
		close(b.confirmationsStopCh)
		b.confirmationsStopCh = nil
	}
	if b.cardsFarmer != nil {
		b.cardsFarmer.Stop()
	}
	if b.steamClient != nil {
		b.steamClient.Disconnect()
	}
}

// SubmitInput 提交登录验证码（Steam Guard/2FA），设置后触发自动重连。
// 供聊天命令 !input 与 IPC /api/bot/{name}/input 使用。
func (b *Bot) SubmitInput(code string) error {
	if b.steamClient == nil {
		return fmt.Errorf("Steam 客户端未就绪")
	}
	if strings.TrimSpace(code) == "" {
		return fmt.Errorf("验证码为空")
	}
	b.steamClient.SetAuthCode(strings.TrimSpace(code))
	go b.scheduleReconnect()
	return nil
}

// scheduleReconnect 在断线后按指数退避自动重连（最多 5 次，间隔 1s/2s/4s/.../60s）。
func (b *Bot) scheduleReconnect() {
	if !b.reconnecting.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer b.reconnecting.Store(false)
		delay := time.Second
		if b.GlobalConfig != nil && b.GlobalConfig.LoginLimiterDelay > 0 {
			delay = time.Duration(b.GlobalConfig.LoginLimiterDelay) * time.Second
		}
		for attempt := 1; attempt <= 5; attempt++ {
			time.Sleep(delay)
			b.Log.Info("Bot %s 尝试自动重连（第 %d 次）", b.Name, attempt)
			b.stopModules()
			if err := b.Start(); err == nil {
				b.Log.Info("Bot %s 重连成功", b.Name)
				return
			} else {
				b.Log.Warn("Bot %s 重连失败: %v", b.Name, err)
			}
			delay *= 2
			if delay > 60*time.Second {
				delay = 60 * time.Second
			}
		}
		b.Log.Warn("Bot %s 达到最大重连次数，停止重连", b.Name)
	}()
}

// Pause 暂停挂机。

func (b *Bot) Pause() {
	if b.cardsFarmer != nil {
		b.cardsFarmer.Stop()
	}
	b.Log.Info("Bot %s 已暂停挂机", b.Name)
}

// Resume 恢复挂机。

func (b *Bot) Resume() {
	if b.cardsFarmer != nil {
		b.cardsFarmer.Start()
	}
	b.Log.Info("Bot %s 已恢复挂机", b.Name)
}

// IsConnected 返回 Steam 登录状态。

func (b *Bot) IsConnected() bool {
	return b.steamClient != nil && b.steamClient.LoggedOn()
}

// WebSessionOrNil 返回当前 Web 会话（未登录时为 nil）。

func (b *Bot) WebSessionOrNil() *steam.WebSession {
	if b.steamClient == nil {
		return nil
	}
	return b.steamClient.WebSessionOrNil()
}

// SteamID 返回当前 Bot 的 SteamID64（未登录时为 0）。

func (b *Bot) SteamID() uint64 {
	if b.steamClient == nil {
		return 0
	}
	return b.steamClient.SteamID()
}

// BadgeCandidatesFromAPI 使用 Steam Web API 获取"徽章未完成"的游戏 AppID 列表。

func (b *Bot) saveConfig() error {
	path := filepath.Join(b.configDir, b.Name+".json")
	if err := b.Config.Save(path); err != nil {
		return fmt.Errorf("保存 Bot 配置失败: %w", err)
	}
	return nil
}

// isLootable 判断物品是否属于可转移类型（交易卡片/闪卡/补充包）。

func (b *Bot) String() string {
	return fmt.Sprintf("Bot(%s)", b.Name)
}

// loginKeyPath 返回登录 Key 持久化文件路径。
