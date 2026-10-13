// Package steam 的 Web API 客户端：基于 github.com/gofurry/steam-go 开源库。
package steam

import (
	"context"
	"fmt"
	"time"

	gosteam "github.com/gofurry/steam-go"
)

// FetchOwnedGames 通过 Steam Web API 获取指定 SteamID 拥有的游戏，返回 map[appid]游戏名。
// 需要 Steam Web API Key。
func FetchOwnedGames(apiKey, steamID string) (map[uint32]string, error) {
	client, err := gosteam.NewClient(
		gosteam.WithAPIKey(apiKey),
		gosteam.WithTimeout(15*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Steam API 客户端失败: %w", err)
	}
	defer client.Close()

	resp, err := client.API.PlayerService.GetOwnedGames(context.Background(), steamID, nil)
	if err != nil {
		return nil, fmt.Errorf("GetOwnedGames 请求失败: %w", err)
	}

	games := make(map[uint32]string, len(resp.Response.Games))
	for _, game := range resp.Response.Games {
		games[game.AppID] = game.Name
	}
	return games, nil
}
// FetchIncompleteBadgeAppIDs 通过 Steam Web API 获取"徽章未完成"（可能有卡牌掉落）的游戏 AppID 列表。
// 需要 Steam Web API Key（https://steamcommunity.com/dev/apikey）。
func FetchIncompleteBadgeAppIDs(apiKey, steamID string) ([]uint32, error) {
	client, err := gosteam.NewClient(
		gosteam.WithAPIKey(apiKey),
		gosteam.WithTimeout(15*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Steam API 客户端失败: %w", err)
	}
	defer client.Close()

	resp, err := client.API.PlayerService.GetBadges(context.Background(), steamID)
	if err != nil {
		return nil, fmt.Errorf("GetBadges 请求失败: %w", err)
	}

	var appIDs []uint32
	for _, badge := range resp.Response.Badges {
		// completion_time == 0 表示徽章未完成，仍可能有卡牌掉落
		if badge.AppID > 0 && badge.CompletionTime == 0 {
			appIDs = append(appIDs, badge.AppID)
		}
	}
	return appIDs, nil
}