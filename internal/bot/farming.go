package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/asf-go/asf/internal/inventory"
	"github.com/asf-go/asf/internal/steam"
	"github.com/asf-go/asf/internal/store"
)

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
		return nil, fmt.Errorf("web 会话不可用: %w", err)
	}
	steamID := b.steamClient.SteamID()
	if steamID == 0 {
		return nil, fmt.Errorf("Bot %s SteamID 无效", b.Name)
	}
	return inventory.GetOwnInventory(ws, steamID, appID, contextID)
}

// startConfirmations 启动确认自动处理循环（每分钟一次，需要已导入移动令牌）。

func (b *Bot) UnpackBoosters() (int, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return 0, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws, err := b.steamClient.WebSession()
	if err != nil {
		return 0, fmt.Errorf("web 会话不可用: %w", err)
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

func fetchSteamLevel(b *Bot, steamID uint64) (int, error) {
	if b.SteamAPIKey == "" {
		return 0, fmt.Errorf("未配置 Steam Web API Key")
	}
	reqURL := fmt.Sprintf("https://api.steampowered.com/IPlayerService/GetSteamLevel/v1/?key=%s&steamid=%d", b.SteamAPIKey, steamID)
	resp, err := http.Get(reqURL)
	if err != nil {
		return 0, fmt.Errorf("查询等级失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
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

// FarmingBlacklistClear 清空挂机黑名单并保存配置。
func (b *Bot) FarmingBlacklistClear() error {
	b.Config.FarmingBlacklist = nil
	return b.saveConfig()
}

// FarmingQueueClear 清空挂机队列并保存配置。
func (b *Bot) FarmingQueueClear() error {
	b.Config.FarmingQueue = nil
	return b.saveConfig()
}
