// Package inventory 实现库存查询：基于 github.com/cyqmq/go-steam/v3/economy/inventory 开源库。
package inventory

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cyqmq/go-steam/v3/economy/inventory"

	"github.com/asf-go/asf/internal/steam"
)

// Item 是库存条目（含描述信息）的简化表示。
type Item struct {
	ID             uint64 `json:"id"`
	ClassID        uint64 `json:"classid"`
	InstanceID     uint64 `json:"instanceid"`
	Amount         uint64 `json:"amount"`
	Name           string `json:"name"`
	MarketName     string `json:"market_name"`
	MarketHashName string `json:"market_hash_name"`
	Type           string `json:"type"`
	IconURL        string `json:"icon_url"`
	Tradable       bool   `json:"tradable"`
	Marketable     bool   `json:"marketable"`
	Tags           string `json:"tags"`
}

// Response 是库存查询的返回结果。
type Response struct {
	AppID     uint32 `json:"appid"`
	ContextID uint64 `json:"contextid"`
	Count     int    `json:"count"`
	Items     []Item `json:"items"`
}

// GetOwnInventory 查询指定 Bot 的自身库存。
// appID 例如 753（Steam 社区物品）或 730（CS2）；contextID 例如 6（交易卡片）或 2（CS2 库存）。
func GetOwnInventory(ws *steam.WebSession, steamID uint64, appID uint32, contextID uint64) (*Response, error) {
	if ws == nil {
		return nil, fmt.Errorf("Web 会话为空")
	}
	inv, err := inventory.GetOwnInventory(ws.HTTPClient(), contextID, appID)
	if err != nil {
		return nil, fmt.Errorf("获取库存失败: %w", err)
	}

	resp := &Response{AppID: appID, ContextID: contextID}
	keys := make([]string, 0, len(inv.Items))
	for id := range inv.Items {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	for _, id := range keys {
		item := inv.Items[id]
		desc := inv.Descriptions[fmt.Sprintf("%d_%d", item.ClassId, item.InstanceId)]
		out := Item{
			ID:         item.Id,
			ClassID:    item.ClassId,
			InstanceID: item.InstanceId,
			Amount:    item.Amount,
		}
		if desc != nil {
			out.Name = desc.Name
			out.MarketName = desc.MarketName
			out.MarketHashName = desc.MarketHashName
			out.Type = desc.Type
			out.IconURL = desc.IconUrl
			out.Tradable = bool(desc.Tradable)
			out.Marketable = bool(desc.Marketable)
			var tags []string
			for _, t := range desc.Tags {
				if t.Name != "" {
					tags = append(tags, t.Name)
				}
			}
			out.Tags = strings.Join(tags, ", ")
		}
		resp.Items = append(resp.Items, out)
	}
	resp.Count = len(resp.Items)
	return resp, nil
}