package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/inventory"
	"github.com/asf-go/asf/internal/trading"
)

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
		// 交易报价检查周期（分钟），对应原版 TradeCheckPeriod
		checkPeriod := time.Duration(b.Config.TradeCheckPeriod) * time.Minute
		if checkPeriod == 0 {
			checkPeriod = 60 * time.Minute
		}
		ticker := time.NewTicker(checkPeriod)
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

	// MaxTradeHoldDuration：拒绝 escrow hold 超过配置上限的报价
	if b.GlobalConfig != nil && b.GlobalConfig.MaxTradeHoldDuration > 0 && offer.EscrowEndDate > 0 {
		holdEnd := time.Unix(int64(offer.EscrowEndDate), 0)
		hold := time.Until(holdEnd)
		if hold > 0 && hold.Hours() >= float64(b.GlobalConfig.MaxTradeHoldDuration)*24 {
			b.Log.Info("报价 %d 的 escrow hold 约 %d 天，超过上限 %d 天，拒绝",
				offer.TradeOfferID, int(hold.Hours()/24), b.GlobalConfig.MaxTradeHoldDuration)
			if err := trading.DeclineTradeOffer(b.steamClient.WebSessionOrNil(), offer.TradeOfferID); err != nil {
				b.Log.Warn("拒绝报价 %d 失败: %v", offer.TradeOfferID, err)
			}
			return
		}
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
	case prefs.Has(config.TradingPreferencesMatchMakingOnly):
		// 匹配交易：对方有提供物品，且我方给出的所有物品类型都在 MatchableTypes 中
		if len(offer.ItemsToReceive) > 0 {
			matchable, err := b.itemsMatchable(offer.ItemsToGive)
			if err != nil {
				b.Log.Warn("检查匹配物品失败: %v", err)
				return
			}
			if matchable {
				shouldAccept = true
				reason = "匹配交易"
			}
		}
	case prefs.Has(config.TradingPreferencesSteamTradeMatcher):
		// SteamTradeMatcher：原版 Exchange 模块的公平交换逻辑
		if len(offer.ItemsToGive) > len(offer.ItemsToReceive) {
			b.Log.Info("STM 拒绝报价 %d（我方给出物品数量更多）", offer.TradeOfferID)
			break
		}
		if len(b.Config.MatchableTypes) == 0 {
			b.Log.Info("STM 忽略报价 %d（MatchableTypes 为空）", offer.TradeOfferID)
			break
		}
		fair, err := b.isFairExchange(offer.ItemsToGive, offer.ItemsToReceive, b.Config.MatchableTypes)
		if err != nil {
			b.Log.Warn("STM 公平性检查失败: %v", err)
			return
		}
		if fair {
			shouldAccept = true
			reason = "STM 公平交换"
		}
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

// itemsMatchable 检查报价中我方给出的所有物品类型是否都在 MatchableTypes 中。
// 通过库存 ClassID 到物品类型的映射判断。
func (b *Bot) itemsMatchable(items []trading.TradeAsset) (bool, error) {
	if len(items) == 0 {
		return true, nil
	}

	type inventoryKey struct {
		appID     uint32
		contextID uint64
	}
	classIDsByKey := make(map[inventoryKey][]uint64)
	for _, it := range items {
		ctx, err := strconv.ParseUint(it.ContextID, 10, 64)
		if err != nil {
			return false, fmt.Errorf("解析 contextID %q 失败: %w", it.ContextID, err)
		}
		key := inventoryKey{appID: it.AppID, contextID: ctx}
		classIDsByKey[key] = append(classIDsByKey[key], it.ClassID)
	}

	for key, classIDs := range classIDsByKey {
		inv, err := b.GetInventory(key.appID, key.contextID)
		if err != nil {
			return false, err
		}
		typeByClass := make(map[uint64]string, len(inv.Items))
		for _, item := range inv.Items {
			typeByClass[item.ClassID] = item.Type
		}
		for _, classID := range classIDs {
			if !matchableType(b.Config.MatchableTypes, typeByClass[classID]) {
				return false, nil
			}
		}
	}
	return true, nil
}

// matchableType 判断物品类型是否在可匹配类型列表中（忽略空格，如 "Trading Card" -> "TradingCard"）。
func matchableType(matchable []config.AssetType, itemType string) bool {
	normalized := strings.ReplaceAll(itemType, " ", "")
	for _, mt := range matchable {
		if string(mt) == normalized {
			return true
		}
	}
	return false
}

// assetTypeKey 标识物品类型（appID + 规范化类型名）。
type assetTypeKey struct {
	appID    uint32
	itemType string
}

// assetTypes 将报价物品按 (appID, 类型) 分组统计数量。
func (b *Bot) assetTypes(items []trading.TradeAsset) (map[assetTypeKey]uint32, error) {
	result := make(map[assetTypeKey]uint32)
	if len(items) == 0 {
		return result, nil
	}

	type inventoryKey struct {
		appID     uint32
		contextID uint64
	}
	classIDsByKey := make(map[inventoryKey][]uint64)
	for _, it := range items {
		ctx, err := strconv.ParseUint(it.ContextID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("解析 contextID %q 失败: %w", it.ContextID, err)
		}
		key := inventoryKey{appID: it.AppID, contextID: ctx}
		classIDsByKey[key] = append(classIDsByKey[key], it.ClassID)
	}

	typeByClass := make(map[uint64]string)
	for key := range classIDsByKey {
		inv, err := b.GetInventory(key.appID, key.contextID)
		if err != nil {
			return nil, err
		}
		for _, item := range inv.Items {
			typeByClass[item.ClassID] = item.Type
		}
	}

	for _, it := range items {
		typ := strings.ReplaceAll(typeByClass[it.ClassID], " ", "")
		key := assetTypeKey{appID: it.AppID, itemType: typ}
		result[key] += uint32(it.Amount)
	}
	return result, nil
}

// isFairExchange 实现原版 Exchange 模块的公平性检查：
// 我方给出的每个 (appID, 类型) 物品数量必须不超过对方提供的同类型数量，且给出的类型必须在 MatchableTypes 中。
func (b *Bot) isFairExchange(give, receive []trading.TradeAsset, matchable []config.AssetType) (bool, error) {
	giveTypes, err := b.assetTypes(give)
	if err != nil {
		return false, err
	}
	receiveTypes, err := b.assetTypes(receive)
	if err != nil {
		return false, err
	}
	return fairExchange(giveTypes, receiveTypes, matchable), nil
}

// fairExchange 是 isFairExchange 的纯逻辑部分（便于单元测试）。
func fairExchange(giveTypes, receiveTypes map[assetTypeKey]uint32, matchable []config.AssetType) bool {
	for key, giveAmount := range giveTypes {
		if !matchableType(matchable, key.itemType) {
			return false
		}
		if receiveTypes[key] < giveAmount {
			return false
		}
	}
	return true
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

func isLootable(item inventory.Item) bool {
	t := strings.ReplaceAll(item.Type, " ", "")
	switch t {
	case "TradingCard", "FoilTradingCard", "BoosterPack":
		return true
	}
	return false
}

// fetchSteamLevel 通过 Web API 查询指定 SteamID 的等级。
