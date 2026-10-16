// Package trading 实现交易报价的查询与处理：使用 Steam IEconService Web API。
package trading

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cyqmq/go-steam/v3/steamid"
	"github.com/cyqmq/go-steam/v3/tradeoffer"

	"github.com/asf-go/asf/internal/steam"
)

// TradeOfferState 对应 Steam 的 ETradeOfferState。
type TradeOfferState int

const (
	TradeOfferStateInvalid                  TradeOfferState = 1
	TradeOfferStateActive                   TradeOfferState = 2
	TradeOfferStateAccepted                 TradeOfferState = 3
	TradeOfferStateCountered                TradeOfferState = 4
	TradeOfferStateExpired                  TradeOfferState = 5
	TradeOfferStateCanceled                 TradeOfferState = 6
	TradeOfferStateDeclined                 TradeOfferState = 7
	TradeOfferStateInvalidItems             TradeOfferState = 8
	TradeOfferStateCreatedNeedsConfirmation TradeOfferState = 9
	TradeOfferStateCanceledBySecondFactor   TradeOfferState = 10
	TradeOfferStateInEscrow                 TradeOfferState = 11
)

// TradeAsset 是交易报价中的物品条目。
type TradeAsset struct {
	AppID      uint32 `json:"appid"`
	ContextID  string `json:"contextid"`
	AssetID    uint64 `json:"assetid,string"`
	ClassID    uint64 `json:"classid,string"`
	InstanceID uint64 `json:"instanceid,string"`
	Amount     uint64 `json:"amount"`
	Missing    bool   `json:"missing"`
}

// TradeOffer 是一条交易报价。
type TradeOffer struct {
	TradeOfferID   uint64          `json:"tradeofferid,string"`
	AccountIDOther uint32          `json:"accountid_other"`
	Message        string          `json:"message"`
	State          TradeOfferState `json:"trade_offer_state"`
	IsOurOffer     bool            `json:"is_our_offer"`
	TimeCreated    uint32          `json:"time_created"`
	TimeUpdated    uint32          `json:"time_updated"`
	ItemsToGive    []TradeAsset    `json:"items_to_give"`
	ItemsToReceive []TradeAsset    `json:"items_to_receive"`
	EscrowEndDate  uint32          `json:"escrow_end_date"`
}

// GetTradeOffersResponse 是 IEconService/GetTradeOffers/v1 的响应。
type GetTradeOffersResponse struct {
	Response struct {
		TradeOffersReceived []TradeOffer `json:"trade_offers_received"`
		TradeOffersSent     []TradeOffer `json:"trade_offers_sent"`
	} `json:"response"`
}

// GetTradeOffers 获取交易报价。需要 Steam Web API Key。
// activeOnly 为 true 时只返回进行中的报价。返回值为（收到的报价，发出的报价）。
func GetTradeOffers(apiKey string, activeOnly bool) ([]TradeOffer, []TradeOffer, error) {
	if apiKey == "" {
		return nil, nil, fmt.Errorf("未配置 Steam Web API Key，无法查询交易报价")
	}
	form := url.Values{}
	form.Set("key", apiKey)
	form.Set("get_received_offers", "1")
	form.Set("get_sent_offers", "1")
	if activeOnly {
		form.Set("active_only", "1")
	}
	form.Set("language", "english")

	reqURL := "https://api.steampowered.com/IEconService/GetTradeOffers/v1/?" + form.Encode()
	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, nil, fmt.Errorf("请求交易报价失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("交易报价接口返回状态码 %d: %s", resp.StatusCode, string(body))
	}
	var out GetTradeOffersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, nil, fmt.Errorf("解析交易报价响应失败: %w", err)
	}
	return out.Response.TradeOffersReceived, out.Response.TradeOffersSent, nil
}

// AcceptTradeOffer 接受指定交易报价（需要 Web 会话 Cookie）。
func AcceptTradeOffer(ws *steam.WebSession, tradeOfferID uint64) error {
	return sendTradeOfferAction(ws, tradeOfferID, "accept")
}

// DeclineTradeOffer 拒绝指定交易报价（需要 Web 会话 Cookie）。
func DeclineTradeOffer(ws *steam.WebSession, tradeOfferID uint64) error {
	return sendTradeOfferAction(ws, tradeOfferID, "decline")
}

// CancelTradeOffer 取消我方发出的指定交易报价（需要 Web 会话 Cookie）。
func CancelTradeOffer(ws *steam.WebSession, tradeOfferID uint64) error {
	return sendTradeOfferAction(ws, tradeOfferID, "cancel")
}

func sendTradeOfferAction(ws *steam.WebSession, tradeOfferID uint64, action string) error {
	if ws == nil {
		return fmt.Errorf("web 会话为空")
	}
	reqURL := fmt.Sprintf("https://steamcommunity.com/tradeoffer/%d/%s", tradeOfferID, action)
	form := url.Values{}
	form.Set("tradeofferid", strconv.FormatUint(tradeOfferID, 10))
	form.Set("serverid", "1")

	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("构造 %s 请求失败: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ws.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("%s 交易报价请求失败: %w", action, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 %s 响应失败: %w", action, err)
	}
	// AcceptTradeOffer 返回 {"response":{"trade_offer_state":N}}；非 JSON 或状态码异常视为失败。
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s 交易报价返回状态码 %d: %s", action, resp.StatusCode, string(body))
	}
	var out struct {
		Response struct {
			TradeOfferState TradeOfferState `json:"trade_offer_state"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("%s 交易报价响应解析失败: %s", action, string(body))
	}
	switch action {
	case "accept":
		if out.Response.TradeOfferState != TradeOfferStateAccepted && out.Response.TradeOfferState != TradeOfferStateInEscrow {
			return fmt.Errorf("%s 交易报价失败，状态码 %d", action, out.Response.TradeOfferState)
		}
	case "decline", "cancel":
		if out.Response.TradeOfferState != TradeOfferStateCanceled && out.Response.TradeOfferState != TradeOfferStateDeclined {
			return fmt.Errorf("%s 交易报价失败，状态码 %d", action, out.Response.TradeOfferState)
		}
	default:
		return fmt.Errorf("未知操作 %s", action)
	}
	return nil
}

// TradeItem 是发送交易报价时的物品条目。
type TradeItem struct {
	AppID     uint32
	ContextID uint64
	AssetID   uint64
	Amount    uint64
}

// SendTradeOffer 向目标用户发送交易报价（我方给出 items，无需对方物品），返回报价 ID。
// 需要 Steam Web API Key（用于创建报价）与已初始化的 Web 会话（身份 Cookie）。
func SendTradeOffer(ws *steam.WebSession, apiKey string, targetSteamID uint64, items []TradeItem, message string) (uint64, error) {
	if ws == nil {
		return 0, fmt.Errorf("web 会话为空")
	}
	if apiKey == "" {
		return 0, fmt.Errorf("未配置 Steam Web API Key，无法发送交易报价")
	}
	if len(items) == 0 {
		return 0, fmt.Errorf("报价物品为空")
	}

	client := tradeoffer.NewClient(tradeoffer.APIKey(apiKey), ws.SessionID(), "", ws.SteamLoginSecure())

	myItems := make([]tradeoffer.TradeItem, 0, len(items))
	for _, item := range items {
		if item.AssetID == 0 {
			return 0, fmt.Errorf("物品 AssetID 无效")
		}
		myItems = append(myItems, tradeoffer.TradeItem{
			AppId:     item.AppID,
			ContextId: item.ContextID,
			AssetId:   item.AssetID,
			Amount:    item.Amount,
		})
	}

	offerID, err := client.Create(steamid.SteamId(targetSteamID), nil, myItems, nil, nil, message)
	if err != nil {
		return 0, fmt.Errorf("发送交易报价失败: %w", err)
	}
	return offerID, nil
}
