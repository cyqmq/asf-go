// Package bot 移植自 ArchiSteamFarm.Steam.Bot（C# 核心 Bot 逻辑的精简版）。
package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/authenticator"
	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/inventory"
	"github.com/asf-go/asf/internal/logging"
	"github.com/asf-go/asf/internal/steam"
	"github.com/asf-go/asf/internal/store"
	"github.com/asf-go/asf/internal/trading"
	"github.com/asf-go/asf/internal/web"
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

	configDir   string
	loginKey    string
	steamClient *steam.Client
	cardsFarmer *CardsFarmer
	auth        *authenticator.Authenticator

	tradingStopCh       chan struct{}
	tradingDoneCh       chan struct{}
	confirmationsStopCh chan struct{}
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
	return b, nil
}

// Start 连接 Steam 并登录。
func (b *Bot) Start() error {
	b.Log.Info("正在启动 Bot %s", b.Name)
	if !b.Config.Enabled {
		b.Log.Info("Bot 未启用，跳过登录")
		return nil
	}
	if b.Config.SteamLogin == "" {
		return fmt.Errorf("Bot %s 未配置 SteamLogin", b.Name)
	}

	client := steam.NewClient(b.Log, b.Config.SteamLogin, b.Config.SteamPassword, b.loginKey)
	client.SetOnLoginKey(func(key string) {
		b.loginKey = key
		b.saveLoginKey()
	})
	client.SetOnChatMessage(b.handleCommand)
	client.SetOnFriendRequest(b.handleFriendRequest)
	client.SetOnChatInvite(b.handleChatInvite)
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

	// 初始化 Web 会话（卡牌挂机需要）
	if err := client.InitWebSession(); err != nil {
		b.Log.Warn("Web 会话初始化失败: %v", err)
	}

	// 启动卡牌挂机
	b.cardsFarmer = NewCardsFarmer(b)
	b.cardsFarmer.Start()

	// 启动交易报价处理（若启用）
	b.startTrading()

	// 启动确认自动处理（若已导入移动令牌）
	b.startConfirmations()

	b.Log.Info("Bot %s 已就绪", b.Name)
	return nil
}

// Stop 断开 Steam 连接。
func (b *Bot) Stop() error {
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
func (b *Bot) BadgeCandidatesFromAPI() ([]uint32, error) {
	if b.SteamAPIKey == "" {
		return nil, fmt.Errorf("Bot %s 未配置 SteamWebApiKey", b.Name)
	}
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return nil, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	steamID := fmt.Sprintf("%d", b.steamClient.SteamID())
	if steamID == "0" {
		return nil, fmt.Errorf("Bot %s SteamID 无效", b.Name)
	}
	return steam.FetchIncompleteBadgeAppIDs(b.SteamAPIKey, steamID)
}

// GetInventory 查询本 Bot 的自身库存（appID 如 753 社区物品，contextID 如 6 交易卡片）。
func (b *Bot) GetInventory(appID uint32, contextID uint64) (*inventory.Response, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return nil, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws, err := b.steamClient.WebSession()
	if err != nil {
		return nil, fmt.Errorf("Web 会话不可用: %w", err)
	}
	steamID := b.steamClient.SteamID()
	if steamID == 0 {
		return nil, fmt.Errorf("Bot %s SteamID 无效", b.Name)
	}
	return inventory.GetOwnInventory(ws, steamID, appID, contextID)
}

// startConfirmations 启动确认自动处理循环（每分钟一次，需要已导入移动令牌）。
func (b *Bot) startConfirmations() {
	if b.auth == nil {
		return
	}
	b.confirmationsStopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				b.processConfirmations()
			case <-b.confirmationsStopCh:
				return
			}
		}
	}()
}

// processConfirmations 查询并自动确认所有待确认项（交易/市场挂单）。
func (b *Bot) processConfirmations() {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return
	}
	ws := b.steamClient.WebSessionOrNil()
	if ws == nil {
		return
	}
	code, err := b.auth.GenerateConfirmationCode("conf")
	if err != nil {
		b.Log.Warn("生成确认码失败: %v", err)
		return
	}
	deviceID := b.auth.DeviceID
	if deviceID == "" {
		deviceID = "android:asf-go"
	}
	now := time.Now().Unix()

	confirmations, err := store.GetConfirmations(ws, b.SteamID(), deviceID, code, now)
	if err != nil {
		b.Log.Debug("查询确认列表失败: %v", err)
		return
	}
	if len(confirmations) == 0 {
		return
	}

	b.Log.Info("发现 %d 个待确认项，自动确认", len(confirmations))
	ok, err := store.HandleConfirmations(ws, b.SteamID(), deviceID, code, now, confirmations, true)
	if err != nil {
		b.Log.Warn("自动确认失败: %v", err)
		return
	}
	if ok {
		b.Log.Info("已确认 %d 个待确认项", len(confirmations))
	} else {
		b.Log.Warn("Steam 返回确认失败")
	}
}

// GetConfirmations 查询当前待确认列表（需要已登录且配置了移动验证器）。
func (b *Bot) GetConfirmations() ([]store.Confirmation, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return nil, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.auth == nil {
		return nil, fmt.Errorf("Bot %s 未配置移动验证器", b.Name)
	}
	ws := b.steamClient.WebSessionOrNil()
	if ws == nil {
		return nil, fmt.Errorf("Bot %s 无 Web 会话", b.Name)
	}
	code, err := b.auth.GenerateConfirmationCode("conf")
	if err != nil {
		return nil, fmt.Errorf("生成确认码失败: %w", err)
	}
	deviceID := b.auth.DeviceID
	if deviceID == "" {
		deviceID = "android:asf-go"
	}
	confirmations, err := store.GetConfirmations(ws, b.SteamID(), deviceID, code, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return confirmations, nil
}

// HandleConfirmations 接受或拒绝指定的确认项。
func (b *Bot) HandleConfirmations(accept bool, confirmations []store.Confirmation) (bool, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return false, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.auth == nil {
		return false, fmt.Errorf("Bot %s 未配置移动验证器", b.Name)
	}
	ws := b.steamClient.WebSessionOrNil()
	if ws == nil {
		return false, fmt.Errorf("Bot %s 无 Web 会话", b.Name)
	}
	code, err := b.auth.GenerateConfirmationCode("conf")
	if err != nil {
		return false, fmt.Errorf("生成确认码失败: %w", err)
	}
	deviceID := b.auth.DeviceID
	if deviceID == "" {
		deviceID = "android:asf-go"
	}
	return store.HandleConfirmations(ws, b.SteamID(), deviceID, code, time.Now().Unix(), confirmations, accept)
}

// startTrading 启动交易报价轮询（立即执行一次，之后按 TradingPeriod 小时轮询）。
func (b *Bot) startTrading() {
	if !b.Config.Trading {
		return
	}
	if b.Config.TradingPreferences.Has(config.TradingPreferencesPassive) || b.SteamAPIKey == "" {
		b.Log.Info("交易模块未启用（Passive 或无 API Key），跳过")
		return
	}
	b.tradingStopCh = make(chan struct{})
	b.tradingDoneCh = make(chan struct{})
	go func() {
		defer close(b.tradingDoneCh)
		b.processTrades()
		if b.Config.SendTradePeriod > 0 {
			b.autoSendLoot()
		}
		period := time.Duration(b.Config.TradingPeriod)
		if period == 0 {
			period = 1
		}
		ticker := time.NewTicker(period * time.Hour)
		defer ticker.Stop()

		var sendC <-chan time.Time
		if b.Config.SendTradePeriod > 0 {
			sendPeriod := time.Duration(b.Config.SendTradePeriod)
			sendTicker := time.NewTicker(sendPeriod * time.Hour)
			defer sendTicker.Stop()
			sendC = sendTicker.C
		}

		for {
			select {
			case <-ticker.C:
				b.processTrades()
			case <-sendC:
				b.autoSendLoot()
			case <-b.tradingStopCh:
				return
			}
		}
	}()
}

// autoSendLoot 按 SendTradePeriod 周期自动把可转移物品发送给允许的用户。
func (b *Bot) autoSendLoot() {
	targets := b.lootTargets()
	if len(targets) == 0 {
		b.Log.Info("自动发送战利品：无目标用户，跳过")
		return
	}
	for _, target := range targets {
		offerID, err := b.Loot(target)
		if err != nil {
			b.Log.Info("自动发送战利品给 %d 失败: %v", target, err)
		} else {
			b.Log.Info("自动发送战利品给 %d 成功，报价 %d", target, offerID)
		}
	}
}

// lootTargets 返回自动发送战利品的目标用户列表。
func (b *Bot) lootTargets() []uint64 {
	if len(b.Config.TradingAllowableSteamIDs) > 0 {
		return b.Config.TradingAllowableSteamIDs
	}
	if b.OwnerSteamID != 0 {
		return []uint64{b.OwnerSteamID}
	}
	return nil
}

// stopTrading 停止交易轮询。
func (b *Bot) stopTrading() {
	if b.tradingStopCh == nil {
		return
	}
	close(b.tradingStopCh)
	<-b.tradingDoneCh
	b.tradingStopCh = nil
	b.tradingDoneCh = nil
}

// processTrades 查询并处理收到的交易报价。
func (b *Bot) processTrades() {
	offers, _, err := trading.GetTradeOffers(b.SteamAPIKey, true)
	if err != nil {
		b.Log.Warn("查询交易报价失败: %v", err)
		return
	}
	if len(offers) == 0 {
		return
	}
	b.Log.Info("发现 %d 个进行中的交易报价", len(offers))

	for _, offer := range offers {
		b.handleTradeOffer(offer)
	}
}

// handleTradeOffer 根据配置决定接受/拒绝/忽略一条交易报价。
func (b *Bot) handleTradeOffer(offer trading.TradeOffer) {
	partnerAccountID := offer.AccountIDOther
	partnerSteamID := uint64(0x0110000100000000) | uint64(partnerAccountID)
	prefs := b.Config.TradingPreferences
	b.Log.Info("收到报价 %d（对方 SteamID %d，消息: %q）", offer.TradeOfferID, partnerSteamID, offer.Message)
	b.notifyOwner(fmt.Sprintf("Bot %s 收到新交易报价 %d（对方 %d）", b.Name, offer.TradeOfferID, partnerSteamID))

	// 黑名单：直接拒绝
	if containsUint64(b.Config.TradingBlacklist, partnerSteamID) {
		b.Log.Info("报价 %d 来自黑名单，拒绝", offer.TradeOfferID)
		if err := trading.DeclineTradeOffer(b.steamClient.WebSessionOrNil(), offer.TradeOfferID); err != nil {
			b.Log.Warn("拒绝报价 %d 失败: %v", offer.TradeOfferID, err)
		}
		return
	}

	// 允许列表：非空且不在列表中则拒绝
	if len(b.Config.TradingAllowableSteamIDs) > 0 && !containsUint64(b.Config.TradingAllowableSteamIDs, partnerSteamID) {
		b.Log.Info("报价 %d 不在允许列表，拒绝", offer.TradeOfferID)
		if err := trading.DeclineTradeOffer(b.steamClient.WebSessionOrNil(), offer.TradeOfferID); err != nil {
			b.Log.Warn("拒绝报价 %d 失败: %v", offer.TradeOfferID, err)
		}
		return
	}

	// Passive：仅记录
	if prefs.Has(config.TradingPreferencesPassive) {
		b.Log.Info("Passive 模式，仅记录报价 %d", offer.TradeOfferID)
		return
	}

	// 礼物（我方无物品、对方有物品）
	isGift := len(offer.ItemsToGive) == 0 && len(offer.ItemsToReceive) > 0
	acceptGifts := b.Config.AcceptGifts || prefs.Has(config.TradingPreferencesAcceptGifts)

	shouldAccept := false
	reason := ""
	switch {
	case prefs.Has(config.TradingPreferencesAcceptAll):
		shouldAccept = true
		reason = "AcceptAll"
	case acceptGifts && isGift:
		shouldAccept = true
		reason = "礼物"
	case prefs.Has(config.TradingPreferencesSteamlvlLimit):
		// SteamlvlLimit：需要查询对方等级；无 API Key 时忽略该限制
		level, err := fetchSteamLevel(b, partnerSteamID)
		if err != nil {
			b.Log.Warn("查询 %d 的等级失败: %v", partnerSteamID, err)
			return
		}
		if level >= int(b.Config.TradingSteamLevel) {
			shouldAccept = true
			reason = fmt.Sprintf("Steam 等级 %d >= %d", level, b.Config.TradingSteamLevel)
		}
	}

	if shouldAccept {
		b.Log.Info("接受报价 %d（%s）", offer.TradeOfferID, reason)
		if err := trading.AcceptTradeOffer(b.steamClient.WebSessionOrNil(), offer.TradeOfferID); err != nil {
			b.Log.Warn("接受报价 %d 失败: %v", offer.TradeOfferID, err)
		}
	} else {
		b.Log.Info("忽略报价 %d（不满足接受条件）", offer.TradeOfferID)
	}
}

// containsUint64 判断切片中是否包含目标值。
func containsUint64(list []uint64, target uint64) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// CloseTrades 关闭所有进行中的交易报价（拒绝收到的、取消发出的），返回关闭数量。
func (b *Bot) CloseTrades() (int, error) {
	received, sent, err := trading.GetTradeOffers(b.SteamAPIKey, true)
	if err != nil {
		return 0, fmt.Errorf("查询交易报价失败: %w", err)
	}

	ws := b.steamClient.WebSessionOrNil()
	closed := 0
	for _, offer := range received {
		if err := trading.DeclineTradeOffer(ws, offer.TradeOfferID); err != nil {
			b.Log.Warn("拒绝报价 %d 失败: %v", offer.TradeOfferID, err)
			continue
		}
		closed++
	}
	for _, offer := range sent {
		if err := trading.CancelTradeOffer(ws, offer.TradeOfferID); err != nil {
			b.Log.Warn("取消报价 %d 失败: %v", offer.TradeOfferID, err)
			continue
		}
		closed++
	}
	return closed, nil
}

// SetNickname 修改本 Bot 的 Steam 昵称。
func (b *Bot) SetNickname(name string) error {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if name == "" {
		return fmt.Errorf("昵称不能为空")
	}
	b.steamClient.SetPersonaName(name)
	return nil
}

// handleFriendRequest 处理收到的好友请求。
func (b *Bot) handleFriendRequest(friendID uint64) {
	b.Log.Info("收到好友请求: %d", friendID)

	switch {
	case b.shouldAcceptFriend(friendID):
		b.Log.Info("接受好友请求: %d", friendID)
		b.steamClient.AcceptFriend(friendID)
		b.notifyOwner(fmt.Sprintf("Bot %s 接受了 %d 的好友请求", b.Name, friendID))
	case b.Config.BotBehaviour.Has(config.BotBehaviourRejectInvalidFriendRequests):
		b.Log.Info("拒绝无效好友请求: %d", friendID)
		b.steamClient.RemoveFriend(friendID)
		b.notifyOwner(fmt.Sprintf("Bot %s 拒绝了无效好友请求: %d", b.Name, friendID))
	default:
		b.Log.Info("忽略好友请求: %d（未配置自动接受或拒绝）", friendID)
	}
}

// shouldAcceptFriend 判断是否应接受好友请求。
func (b *Bot) shouldAcceptFriend(friendID uint64) bool {
	if b.Config.AcceptFriendRequests {
		return true
	}
	if _, ok := b.Config.SteamUserPermissions[fmt.Sprintf("%d", friendID)]; ok {
		return true
	}
	if containsUint64(b.Config.TradingAllowableSteamIDs, friendID) {
		return true
	}
	return false
}

// handleChatInvite 处理聊天/群组邀请。
func (b *Bot) handleChatInvite(inviterID, chatRoomID uint64, roomName string) {
	b.Log.Info("收到聊天邀请: %q (来自 %d)", roomName, inviterID)

	if b.Config.BotBehaviour.Has(config.BotBehaviourRejectInvalidGroupInvites) && !b.shouldAcceptFriend(inviterID) {
		b.Log.Info("拒绝无效群组邀请: %q", roomName)
		b.notifyOwner(fmt.Sprintf("Bot %s 拒绝了群组邀请: %s", b.Name, roomName))
		return
	}
	b.Log.Info("接受聊天邀请并加入: %q", roomName)
	b.steamClient.JoinChat(chatRoomID)
}

// notifyOwner 向 SteamOwnerID 发送通知消息（若 owner 为好友则成功，否则仅记录日志）。
func (b *Bot) notifyOwner(message string) {
	if b.OwnerSteamID == 0 {
		return
	}
	b.Log.Info("通知 Owner: %s", message)
	if b.steamClient != nil && b.steamClient.LoggedOn() {
		b.steamClient.SendChatMessage(b.OwnerSteamID, message)
	}
}

// Loot 将库存中的可转移物品（卡片/闪卡/补充包）发送给指定用户，返回报价 ID。
func (b *Bot) Loot(targetSteamID uint64) (uint64, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return 0, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws, err := b.steamClient.WebSession()
	if err != nil {
		return 0, fmt.Errorf("Web 会话不可用: %w", err)
	}
	steamID := b.steamClient.SteamID()
	if steamID == 0 {
		return 0, fmt.Errorf("Bot %s SteamID 无效", b.Name)
	}

	inv, err := inventory.GetOwnInventory(ws, steamID, 753, 6)
	if err != nil {
		return 0, fmt.Errorf("获取库存失败: %w", err)
	}

	var items []trading.TradeItem
	for _, item := range inv.Items {
		if !isLootable(item) {
			continue
		}
		items = append(items, trading.TradeItem{
			AppID:     753,
			ContextID: 6,
			AssetID:   item.ID,
			Amount:    item.Amount,
		})
	}
	if len(items) == 0 {
		return 0, fmt.Errorf("库存中没有可转移物品")
	}

	offerID, err := trading.SendTradeOffer(ws, b.SteamAPIKey, targetSteamID, items, fmt.Sprintf("Loot from %s", b.Name))
	if err != nil {
		return 0, fmt.Errorf("发送交易报价失败: %w", err)
	}
	b.Log.Info("已发送报价 %d 给 %d，共 %d 个物品", offerID, targetSteamID, len(items))
	return offerID, nil
}

// AddLicense 激活免费许可。支持 sub/<subid>、app/<appid> 或商店链接。
func (b *Bot) AddLicense(identifier string) (string, error) {
	idType, id, ok := ParseGameIdentifier(identifier, GameIdentifierPackage)
	if !ok {
		return "", fmt.Errorf("无效游戏标识符: %s", identifier)
	}
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}

	switch idType {
	case GameIdentifierPackage:
		ws, err := b.steamClient.WebSession()
		if err != nil {
			return "", fmt.Errorf("Web 会话不可用: %w", err)
		}
		result, err := store.AddFreeLicense(ws, uint64(id))
		if err != nil {
			return "", fmt.Errorf("激活失败: %w", err)
		}
		b.Log.Info("已请求激活免费许可 sub/%d: %s", id, result)
		return fmt.Sprintf("sub/%d: %s", id, result), nil
	case GameIdentifierApplication:
		if b.steamClient == nil || !b.steamClient.LoggedOn() {
			return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
		}
		result, err := b.steamClient.RequestFreeLicense(uint32(id))
		if err != nil {
			return "", fmt.Errorf("app/%d 激活失败: %w", id, err)
		}
		b.Log.Info("已请求激活免费许可 app/%d: result=%d, granted_apps=%v, granted_packages=%v", id, result.Result, result.GrantedApps, result.GrantedPackages)
		return fmt.Sprintf("app/%d: 结果 %d，授权应用 %v，授权包 %v", id, result.Result, result.GrantedApps, result.GrantedPackages), nil
	default:
		return "", fmt.Errorf("不支持的游戏标识符类型")
	}
}

// CheckAppBeta 提交指定 appID 的 beta 密码，返回可用 beta 分支及其加密密钥。
func (b *Bot) CheckAppBeta(appID uint32, password string) (string, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	result, err := b.steamClient.CheckAppBetaPassword(appID, password)
	if err != nil {
		return "", fmt.Errorf("beta 检查失败: %w", err)
	}
	var lines []string
	for _, bp := range result.BetaPasswords {
		lines = append(lines, fmt.Sprintf("%s: %x", bp.Name, bp.Password))
	}
	if len(lines) == 0 {
		return fmt.Sprintf("app/%d: 无可用 beta 分支（结果 %d）", appID, result.Result), nil
	}
	return fmt.Sprintf("app/%d: %s", appID, strings.Join(lines, ", ")), nil
}

// Owns 查询本 Bot 是否拥有指定游戏。identifiers 支持 app/<appid> 或裸数字。
func (b *Bot) Owns(identifiers []string) (string, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.SteamAPIKey == "" {
		return "", fmt.Errorf("未配置 SteamWebApiKey，无法查询游戏库")
	}
	steamID := fmt.Sprintf("%d", b.steamClient.SteamID())
	games, err := steam.FetchOwnedGames(b.SteamAPIKey, steamID)
	if err != nil {
		return "", fmt.Errorf("查询游戏库失败: %w", err)
	}

	var lines []string
	for _, ident := range identifiers {
		idType, id, ok := ParseGameIdentifier(ident, GameIdentifierApplication)
		if !ok || idType != GameIdentifierApplication {
			lines = append(lines, fmt.Sprintf("%s: 无效 appid", ident))
			continue
		}
		if name, ok := games[id]; ok {
			lines = append(lines, fmt.Sprintf("app/%d: 已拥有 (%s)", id, name))
		} else {
			lines = append(lines, fmt.Sprintf("app/%d: 未拥有", id))
		}
	}
	b.Log.Info("Bot %s 查询了 %d 个游戏所有权", b.Name, len(identifiers))
	return strings.Join(lines, "; "), nil
}

// Redeem 兑换一个或多个 Steam 激活码，返回每个激活码的结果。
func (b *Bot) Redeem(keys []string) (string, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws, err := b.steamClient.WebSession()
	if err != nil {
		return "", fmt.Errorf("Web 会话不可用: %w", err)
	}

	var lines []string
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			lines = append(lines, "空激活码，跳过")
			continue
		}
		result, err := store.RedeemKey(ws, key)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: 兑换失败: %v", key, err))
			continue
		}
		b.Log.Info("Bot %s 兑换激活码 %s: %s", b.Name, key, result)
		lines = append(lines, fmt.Sprintf("%s: %s", key, result))
	}
	return strings.Join(lines, "; "), nil
}

// PointsBalance 查询本 Bot 的 Steam 积分余额（需要 SteamWebApiKey）。
func (b *Bot) PointsBalance() (int64, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return 0, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.SteamAPIKey == "" {
		return 0, fmt.Errorf("未配置 SteamWebApiKey")
	}
	steamID := fmt.Sprintf("%d", b.steamClient.SteamID())
	return steam.FetchPointsBalance(b.SteamAPIKey, steamID)
}

// Update 检查 asf-go 自身是否有新版本；若有则下载对应平台二进制、替换并重启。
func (b *Bot) Update() (string, error) {
	client := web.NewGitHubClient()
	release, err := client.GetLatestRelease("cyqmq", "asf-go", true)
	if err != nil {
		// asf-go 尚无 Release 时回退到上游 ASF 版本信息
		upstream, upErr := client.GetLatestRelease("JustArchiNET", "ArchiSteamFarm", true)
		if upErr != nil {
			return "", fmt.Errorf("检查更新失败: %w", err)
		}
		return fmt.Sprintf("asf-go 暂无 Release，上游 ASF 最新版本: %s", upstream.TagName), nil
	}

	if web.CompareVersions(release.TagName, b.ASFVersion) <= 0 {
		return fmt.Sprintf("当前已是最新版本 %s", b.ASFVersion), nil
	}

	assetName := fmt.Sprintf("asf-go-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	var asset *web.GitHubAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		return "", fmt.Errorf("新版本 %s 中未找到当前平台更新包 %s", release.TagName, assetName)
	}

	b.Log.Info("发现新版本 %s，正在下载 %s ...", release.TagName, asset.Name)
	tmpFile, err := os.CreateTemp("", "asf-go-update-*")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	if err := client.DownloadAsset(asset, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("下载更新包失败: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("设置执行权限失败: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("定位当前可执行文件失败: %w", err)
	}

	// 备份旧文件再替换，避免替换失败导致程序不可用
	backupPath := exe + ".old"
	_ = os.Remove(backupPath)
	if err := os.Rename(exe, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("备份旧可执行文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Rename(backupPath, exe)
		return "", fmt.Errorf("替换可执行文件失败: %w", err)
	}

	b.Log.Info("更新完成，正在重启 ...")
	cmd := exec.Command(exe)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("重启失败: %w", err)
	}

	if b.OnExit != nil {
		b.OnExit()
	}
	return fmt.Sprintf("已更新到 %s，正在重启", release.TagName), nil
}

// UpdateCheck 检查上游 ASF 是否有新版本。
// UnpackBoosters 打开库存中所有补充包，返回开包数量。
func (b *Bot) UnpackBoosters() (int, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return 0, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws, err := b.steamClient.WebSession()
	if err != nil {
		return 0, fmt.Errorf("Web 会话不可用: %w", err)
	}
	steamID := b.steamClient.SteamID()
	if steamID == 0 {
		return 0, fmt.Errorf("Bot %s SteamID 无效", b.Name)
	}

	inv, err := inventory.GetOwnInventory(ws, steamID, 753, 6)
	if err != nil {
		return 0, fmt.Errorf("获取库存失败: %w", err)
	}

	opened := 0
	for _, item := range inv.Items {
		if strings.ReplaceAll(item.Type, " ", "") != "BoosterPack" {
			continue
		}
		ok, err := store.UnpackBooster(ws, steamID, 753, item.ID)
		if err != nil {
			b.Log.Warn("开补充包 %d 失败: %v", item.ID, err)
			continue
		}
		if ok {
			opened++
			b.Log.Info("已打开补充包 %d", item.ID)
		} else {
			b.Log.Warn("开补充包 %d 返回失败", item.ID)
		}
	}
	return opened, nil
}

// FarmingBlacklistAdd 添加游戏到挂机黑名单并保存配置。
func (b *Bot) FarmingBlacklistAdd(appID uint32) error {
	for _, id := range b.Config.FarmingBlacklist {
		if id == appID {
			return nil
		}
	}
	b.Config.FarmingBlacklist = append(b.Config.FarmingBlacklist, appID)
	return b.saveConfig()
}

// FarmingBlacklistRemove 从挂机黑名单移除游戏并保存配置。
func (b *Bot) FarmingBlacklistRemove(appID uint32) error {
	filtered := make([]uint32, 0, len(b.Config.FarmingBlacklist))
	for _, id := range b.Config.FarmingBlacklist {
		if id != appID {
			filtered = append(filtered, id)
		}
	}
	b.Config.FarmingBlacklist = filtered
	return b.saveConfig()
}

// FarmingQueueAdd 添加游戏到挂机队列并保存配置。
func (b *Bot) FarmingQueueAdd(appID uint32) error {
	for _, id := range b.Config.FarmingQueue {
		if id == appID {
			return nil
		}
	}
	b.Config.FarmingQueue = append(b.Config.FarmingQueue, appID)
	return b.saveConfig()
}

// FarmingQueueRemove 从挂机队列移除游戏并保存配置。
func (b *Bot) FarmingQueueRemove(appID uint32) error {
	filtered := make([]uint32, 0, len(b.Config.FarmingQueue))
	for _, id := range b.Config.FarmingQueue {
		if id != appID {
			filtered = append(filtered, id)
		}
	}
	b.Config.FarmingQueue = filtered
	return b.saveConfig()
}

// saveConfig 将 Bot 配置保存到配置目录。
func (b *Bot) saveConfig() error {
	path := filepath.Join(b.configDir, b.Name+".json")
	if err := b.Config.Save(path); err != nil {
		return fmt.Errorf("保存 Bot 配置失败: %w", err)
	}
	return nil
}

// isLootable 判断物品是否属于可转移类型（交易卡片/闪卡/补充包）。
func isLootable(item inventory.Item) bool {
	t := strings.ReplaceAll(item.Type, " ", "")
	switch t {
	case "TradingCard", "FoilTradingCard", "BoosterPack":
		return true
	}
	return false
}

// fetchSteamLevel 通过 Web API 查询指定 SteamID 的等级。
func fetchSteamLevel(b *Bot, steamID uint64) (int, error) {
	if b.SteamAPIKey == "" {
		return 0, fmt.Errorf("未配置 Steam Web API Key")
	}
	reqURL := fmt.Sprintf("https://api.steampowered.com/IPlayerService/GetSteamLevel/v1/?key=%s&steamid=%d", b.SteamAPIKey, steamID)
	resp, err := http.Get(reqURL)
	if err != nil {
		return 0, fmt.Errorf("查询等级失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("等级接口返回状态码 %d", resp.StatusCode)
	}
	var out struct {
		Response struct {
			PlayerLevel int `json:"player_level"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("解析等级响应失败: %w", err)
	}
	return out.Response.PlayerLevel, nil
}

// checkAccess 检查发送者是否具有指定权限（Owner 始终通过；黑名单用户一律拒绝）。
func (b *Bot) checkAccess(chatterID uint64, min config.Access) bool {
	if containsUint64(b.Blacklist, chatterID) {
		return false
	}
	if b.OwnerSteamID != 0 && chatterID == b.OwnerSteamID {
		return true
	}
	userAccess, ok := b.Config.SteamUserPermissions[fmt.Sprintf("%d", chatterID)]
	if !ok {
		return false
	}
	return userAccess.Rank() >= min.Rank()
}

// handleCommand 处理 Steam 聊天命令（以 CommandPrefix 开头）。
func (b *Bot) handleCommand(chatterID uint64, message string) {
	prefix := b.CommandPrefix
	if prefix == "" {
		prefix = "!"
	}
	if !strings.HasPrefix(message, prefix) {
		return
	}
	command := strings.TrimSpace(strings.TrimPrefix(message, prefix))
	lower := strings.ToLower(command)

	b.Log.Info("收到来自 %d 的命令: %s", chatterID, command)

	var reply string

	switch {
	case lower == "help":
		reply = "可用命令: status, pause, resume, farm, restart, unpack, fb <fbadd/fbrm>, fq <fqadd/fqrm>, loot <SteamID64>, transfer <BotName>, closetrades, nickname <name>, addlicense <sub/xxx>, checkappbeta <appid> <password>, owns <appid>, redeem <key>, level, points, stats, version, update, exit, help"
	case lower == "status":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		reply = b.statusText()
	case lower == "pause":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Pause()
		reply = "挂机已暂停"
	case lower == "resume":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Resume()
		reply = "挂机已恢复"
	case lower == "farm":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Resume()
		reply = "已重新开始挂机"
	case lower == "level":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		level, err := b.SteamLevel()
		if err != nil {
			reply = fmt.Sprintf("查询等级失败: %v", err)
		} else {
			reply = fmt.Sprintf("Steam 等级: %d", level)
		}
	case lower == "stats":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		reply = b.statsText()
	case lower == "version":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		if b.ASFVersion == "" {
			reply = "ASF Go 版"
		} else {
			reply = "ASF Go 版 " + b.ASFVersion
		}
	case lower == "points" || lower == "balance":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		points, err := b.PointsBalance()
		if err != nil {
			reply = fmt.Sprintf("查询积分失败: %v", err)
		} else {
			reply = fmt.Sprintf("Steam 积分: %d", points)
		}
	case lower == "restart":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Pause()
		b.Resume()
		reply = "已重启挂机"
	case lower == "update":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		reply, _ = b.Update()
	case lower == "unpack":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		opened, err := b.UnpackBoosters()
		if err != nil {
			reply = fmt.Sprintf("开补充包失败: %v", err)
		} else {
			reply = fmt.Sprintf("已打开 %d 个补充包", opened)
		}
	case lower == "fb":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		if len(b.Config.FarmingBlacklist) == 0 {
			reply = "挂机黑名单为空"
		} else {
			ids := make([]string, 0, len(b.Config.FarmingBlacklist))
			for _, id := range b.Config.FarmingBlacklist {
				ids = append(ids, strconv.FormatUint(uint64(id), 10))
			}
			reply = "挂机黑名单: " + strings.Join(ids, ", ")
		}
	case strings.HasPrefix(lower, "fbadd "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fbadd"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingBlacklistAdd(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 加入挂机黑名单", appID)
			}
		}
	case strings.HasPrefix(lower, "fbrm "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fbrm"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingBlacklistRemove(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 移出挂机黑名单", appID)
			}
		}
	case lower == "fq":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		if len(b.Config.FarmingQueue) == 0 {
			reply = "挂机队列为空"
		} else {
			ids := make([]string, 0, len(b.Config.FarmingQueue))
			for _, id := range b.Config.FarmingQueue {
				ids = append(ids, strconv.FormatUint(uint64(id), 10))
			}
			reply = "挂机队列: " + strings.Join(ids, ", ")
		}
	case strings.HasPrefix(lower, "fqadd "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fqadd"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingQueueAdd(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 加入挂机队列", appID)
			}
		}
	case strings.HasPrefix(lower, "fqrm "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fqrm"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingQueueRemove(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 移出挂机队列", appID)
			}
		}
	case lower == "exit":
		if b.OwnerSteamID == 0 || chatterID != b.OwnerSteamID {
			reply = "权限不足"
			break
		}
		reply = "正在退出 ASF"
		if b.OnExit != nil {
			b.OnExit()
		}
	case strings.HasPrefix(lower, "loot "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("loot"):])
		targetID, err := strconv.ParseUint(arg, 10, 64)
		if err != nil {
			reply = "无效 SteamID。用法: !loot <SteamID64>"
		} else {
			offerID, err := b.Loot(targetID)
			if err != nil {
				reply = fmt.Sprintf("发送报价失败: %v", err)
			} else {
				reply = fmt.Sprintf("已发送报价 %d", offerID)
			}
		}
	case strings.HasPrefix(lower, "transfer "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("transfer"):])
		targetID := uint64(0)
		if b.FindBotSteamID != nil {
			targetID = b.FindBotSteamID(arg)
		}
		if targetID == 0 {
			reply = fmt.Sprintf("目标 Bot %s 未连接或不存在", arg)
		} else {
			offerID, err := b.Loot(targetID)
			if err != nil {
				reply = fmt.Sprintf("发送报价失败: %v", err)
			} else {
				reply = fmt.Sprintf("已向 %s 发送报价 %d", arg, offerID)
			}
		}
	case lower == "closetrades":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		closed, err := b.CloseTrades()
		if err != nil {
			reply = fmt.Sprintf("关闭交易失败: %v", err)
		} else {
			reply = fmt.Sprintf("已关闭 %d 个交易报价", closed)
		}
	case strings.HasPrefix(lower, "nickname "):
		if !b.checkAccess(chatterID, config.AccessFamilySharing) {
			reply = "权限不足"
			break
		}
		nickname := strings.TrimSpace(command[len("nickname"):])
		if err := b.SetNickname(nickname); err != nil {
			reply = fmt.Sprintf("修改昵称失败: %v", err)
		} else {
			reply = fmt.Sprintf("昵称已修改为: %s", nickname)
		}
	case strings.HasPrefix(lower, "addlicense") || strings.HasPrefix(lower, "addlicence") || strings.HasPrefix(lower, "al"):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		arg := ""
		if lower != "addlicense" && lower != "addlicence" && lower != "al" {
			if idx := strings.Index(command, " "); idx >= 0 {
				arg = strings.TrimSpace(command[idx+1:])
			}
		}
		if arg == "" {
			reply = "用法: !addlicense <sub/xxx | app/xxx | 商店链接>（多个用空格分隔）"
		} else {
			identifiers := strings.Fields(arg)
			lines := make([]string, 0, len(identifiers))
			for _, ident := range identifiers {
				resp, err := b.AddLicense(ident)
				if err != nil {
					lines = append(lines, fmt.Sprintf("%s 失败: %v", ident, err))
				} else {
					lines = append(lines, resp)
				}
			}
			reply = strings.Join(lines, "; ")
		}
	case strings.HasPrefix(lower, "checkappbeta "):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		parts := strings.Fields(command[len("checkappbeta"):])
		if len(parts) < 2 {
			reply = "用法: !checkappbeta <appid> <password>"
		} else {
			appID, err := strconv.ParseUint(parts[0], 10, 32)
			if err != nil {
				reply = "无效 appid"
			} else {
				reply, err = b.CheckAppBeta(uint32(appID), parts[1])
				if err != nil {
					reply = fmt.Sprintf("beta 检查失败: %v", err)
				}
			}
		}
	case strings.HasPrefix(lower, "owns "):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("owns"):])
		if arg == "" {
			reply = "用法: !owns <appid> [appid...]"
		} else {
			resp, err := b.Owns(strings.Fields(arg))
			if err != nil {
				reply = fmt.Sprintf("查询失败: %v", err)
			} else {
				reply = resp
			}
		}
	case strings.HasPrefix(lower, "redeem "):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("redeem"):])
		if arg == "" {
			reply = "用法: !redeem <激活码> [激活码...]"
		} else {
			resp, err := b.Redeem(strings.Fields(arg))
			if err != nil {
				reply = fmt.Sprintf("兑换失败: %v", err)
			} else {
				reply = resp
			}
		}
	default:
		reply = "未知命令。发送 !help 查看帮助。"
	}

	if b.steamClient != nil {
		b.steamClient.SendChatMessage(chatterID, reply)
	}
}

// statusText 生成 Bot 状态文本。
func (b *Bot) statusText() string {
	state := "未连接"
	if b.IsConnected() {
		state = "已登录"
	}
	text := fmt.Sprintf("%s | %s | 在线状态: %s", b.Name, state, b.Config.OnlineStatus)
	games := b.FarmingGames()
	if len(games) == 0 {
		text += " | 当前无挂机游戏"
	} else {
		text += fmt.Sprintf(" | 挂机游戏 %d 个", len(games))
		for _, g := range games {
			text += fmt.Sprintf(" | %s(%d):%d", g.Name, g.AppID, g.CardsRemaining)
		}
	}
	return text
}

// IsFarming 返回卡牌挂机是否正在运行。
func (b *Bot) IsFarming() bool {
	return b.cardsFarmer != nil && b.cardsFarmer.IsRunning()
}

// FarmingGames 返回当前挂机游戏列表。
func (b *Bot) FarmingGames() []GameInfo {
	if b.cardsFarmer == nil {
		return nil
	}
	return b.cardsFarmer.GamesToFarm()
}

// SteamLevel 查询本 Bot 的 Steam 等级（需要 SteamWebApiKey）。
func (b *Bot) SteamLevel() (int, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return 0, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	steamID := b.steamClient.SteamID()
	if steamID == 0 {
		return 0, fmt.Errorf("Bot %s SteamID 无效", b.Name)
	}
	return fetchSteamLevel(b, steamID)
}

// statsText 生成 Bot 统计文本。
func (b *Bot) statsText() string {
	state := "未连接"
	if b.IsConnected() {
		state = "已登录"
	}
	text := fmt.Sprintf("%s | %s | 在线状态: %s", b.Name, state, b.Config.OnlineStatus)
	games := b.FarmingGames()
	if len(games) == 0 {
		text += " | 当前无挂机游戏"
	} else {
		text += fmt.Sprintf(" | 挂机游戏 %d 个", len(games))
		for _, g := range games {
			text += fmt.Sprintf(" | %s(%d):%d", g.Name, g.AppID, g.CardsRemaining)
		}
	}
	text += fmt.Sprintf(" | 交易模块: %v | 好友请求自动接受: %v", b.Config.Trading, b.Config.AcceptFriendRequests)
	return text
}

// String 实现 fmt.Stringer。
func (b *Bot) String() string {
	return fmt.Sprintf("Bot(%s)", b.Name)
}

// loginKeyPath 返回登录 Key 持久化文件路径。
func (b *Bot) loginKeyPath() string {
	return filepath.Join(b.configDir, fmt.Sprintf("%s.loginkey.json", b.Name))
}

// maFilePath 返回移动令牌文件路径。
func (b *Bot) maFilePath() string {
	return filepath.Join(b.configDir, fmt.Sprintf("%s.maFile.json", b.Name))
}

func (b *Bot) loadAuthenticator() *authenticator.Authenticator {
	a, err := authenticator.Load(b.maFilePath())
	if err != nil {
		return nil
	}
	return a
}

// HasAuthenticator 返回是否已配置移动令牌。
func (b *Bot) HasAuthenticator() bool {
	return b.auth != nil
}

// Auth 返回移动令牌实例（未配置时为 nil）。
func (b *Bot) Auth() *authenticator.Authenticator {
	return b.auth
}

// TwoFactorCode 返回当前 Steam Guard 验证码。
func (b *Bot) TwoFactorCode() (string, error) {
	if b.auth == nil {
		return "", fmt.Errorf("Bot %s 未配置移动令牌", b.Name)
	}
	return b.auth.GenerateCode()
}

// ImportAuthenticator 导入并保存移动令牌（shared_secret / identity_secret / 可选 device_id）。
func (b *Bot) ImportAuthenticator(sharedSecret, identitySecret, deviceID string) error {
	a, err := authenticator.Import(sharedSecret, identitySecret, deviceID)
	if err != nil {
		return err
	}
	if err := a.Save(b.maFilePath()); err != nil {
		return err
	}
	b.auth = a
	b.Log.Info("移动令牌已导入并保存")
	return nil
}

type loginKeyFile struct {
	LoginKey string `json:"LoginKey"`
}

func (b *Bot) loadLoginKey() string {
	data, err := os.ReadFile(b.loginKeyPath())
	if err != nil {
		return ""
	}
	var f loginKeyFile
	if err := json.Unmarshal(data, &f); err != nil {
		return ""
	}
	return f.LoginKey
}

func (b *Bot) saveLoginKey() {
	f := loginKeyFile{LoginKey: b.loginKey}
	data, err := json.MarshalIndent(&f, "", "\t")
	if err != nil {
		return
	}
	_ = os.WriteFile(b.loginKeyPath(), data, 0o600)
}
