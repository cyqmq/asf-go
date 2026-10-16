package store

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/asf-go/asf/internal/steam"
)

// ConfirmationType 对应 Steam 的 EMobileConfirmationType。
type ConfirmationType int

const (
	ConfirmationTypeTrade         ConfirmationType = 2
	ConfirmationTypeMarketListing ConfirmationType = 3
)

// FlexUint64 兼容 JSON 数字与数字字符串。
type FlexUint64 uint64

// UnmarshalJSON 支持 "123" 与 123 两种形式。
func (u *FlexUint64) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("解析 uint64 失败: %w", err)
	}
	*u = FlexUint64(v)
	return nil
}

// Confirmation 对应原版 ArchiSteamFarm.Steam.Data.Confirmation。
type Confirmation struct {
	ID        FlexUint64 `json:"id"`
	Nonce     FlexUint64 `json:"nonce"`
	Type      int        `json:"type"`
	CreatorID FlexUint64 `json:"creator_id"`
	TypeName  string     `json:"type_name,omitempty"`
}

// ConfirmationsResponse 是 mobileconf/getlist 的响应。
type ConfirmationsResponse struct {
	Success       bool           `json:"success"`
	Confirmations []Confirmation `json:"conf"`
}

// GetConfirmations 查询待确认列表（对应原版 ArchiWebHandler.GetConfirmations）。
// 需要 Web 会话 Cookie（steamcommunity.com）。
func GetConfirmations(ws *steam.WebSession, steamID uint64, deviceID, confirmationHash string, time int64) ([]Confirmation, error) {
	if ws == nil {
		return nil, fmt.Errorf("web 会话为空")
	}

	query := url.Values{}
	query.Set("a", strconv.FormatUint(steamID, 10))
	query.Set("k", confirmationHash)
	query.Set("l", "english")
	query.Set("m", "react")
	query.Set("p", deviceID)
	query.Set("t", strconv.FormatInt(time, 10))
	query.Set("tag", "conf")

	reqURL := "https://steamcommunity.com/mobileconf/getlist?" + query.Encode()
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造确认列表请求失败: %w", err)
	}
	req.Header.Set("Referer", "https://steamcommunity.com/mobileconf")

	resp, err := ws.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求确认列表失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取确认列表响应失败: %w", err)
	}

	var out ConfirmationsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("解析确认列表响应失败: %w", err)
	}
	return out.Confirmations, nil
}

// HandleConfirmations 确认或拒绝一批确认（对应原版 ArchiWebHandler.HandleConfirmations）。
// accept 为 true 时允许，false 时拒绝。返回 Steam 的 success 字段。
func HandleConfirmations(ws *steam.WebSession, steamID uint64, deviceID, confirmationHash string, time int64, confirmations []Confirmation, accept bool) (bool, error) {
	if ws == nil {
		return false, fmt.Errorf("web 会话为空")
	}
	if len(confirmations) == 0 {
		return false, fmt.Errorf("确认列表为空")
	}

	form := url.Values{}
	form.Set("a", strconv.FormatUint(steamID, 10))
	form.Set("k", confirmationHash)
	form.Set("m", "react")
	if accept {
		form.Set("op", "allow")
	} else {
		form.Set("op", "cancel")
	}
	form.Set("p", deviceID)
	form.Set("t", strconv.FormatInt(time, 10))
	form.Set("tag", "conf")
	for _, c := range confirmations {
		form.Add("cid[]", strconv.FormatUint(uint64(c.ID), 10))
		form.Add("ck[]", strconv.FormatUint(uint64(c.Nonce), 10))
	}

	req, err := http.NewRequest(http.MethodPost, "https://steamcommunity.com/mobileconf/multiajaxop", strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("构造确认请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://steamcommunity.com/mobileconf")

	resp, err := ws.HTTPClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("请求确认操作失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取确认操作响应失败: %w", err)
	}

	var out struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return false, fmt.Errorf("解析确认操作响应失败: %w", err)
	}
	return out.Success, nil
}
