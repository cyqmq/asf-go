// Package store 实现 Steam 商店相关的 Web 操作（对应原版 ArchiWebHandler 的商店部分）。
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

// SteamStoreURL 是商店根地址。
const SteamStoreURL = "https://store.steampowered.com"

// EResult 对应 SteamKit2 的 EResult 枚举（常用值）。
type EResult int

const (
	EResultOK                 EResult = 1
	EResultFail               EResult = 2
	EResultTimeout            EResult = 22
	EResultServiceUnavailable EResult = 31
	EResultAccessDenied       EResult = 84
)

// PurchaseResultDetail 对应 SteamKit2 的 EPurchaseResultDetail 枚举。
// 由于 Steam 返回的 purchaseresultdetail 本身就是数字，直接透传。
type PurchaseResultDetail byte

const (
	EPurchaseResultDetailNoDetail         PurchaseResultDetail = 0
	EPurchaseResultDetailAlreadyPurchased PurchaseResultDetail = 9
	EPurchaseResultDetailInvalidData      PurchaseResultDetail = 14
	EPurchaseResultDetailTimeout          PurchaseResultDetail = 22
	EPurchaseResultDetailInvalidPackage   PurchaseResultDetail = 24
)

// AddFreeLicenseResult 是激活免费许可的结果。
type AddFreeLicenseResult struct {
	Result               EResult
	PurchaseResultDetail PurchaseResultDetail
}

// String 返回便于日志/命令回复的文本。
func (r AddFreeLicenseResult) String() string {
	return fmt.Sprintf("%d/%d", r.Result, r.PurchaseResultDetail)
}

// RedeemKeyResult 是激活码兑换的结果。
type RedeemKeyResult struct {
	Success bool
	Detail  string
}

// String 返回便于命令回复的文本。
func (r RedeemKeyResult) String() string {
	if r.Success {
		return "成功"
	}
	return fmt.Sprintf("失败: %s", r.Detail)
}

// RedeemKey 兑换一个 Steam 激活码（对应原版 ASF 的 RedeemKey，Web 方式等价实现：
// POST https://store.steampowered.com/account/ajaxregisterkey/）。
func RedeemKey(ws *steam.WebSession, key string) (RedeemKeyResult, error) {
	if ws == nil {
		return RedeemKeyResult{}, fmt.Errorf("web 会话为空")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return RedeemKeyResult{}, fmt.Errorf("激活码为空")
	}

	reqURL := fmt.Sprintf("%s/account/ajaxregisterkey/", SteamStoreURL)
	form := url.Values{}
	form.Set("product_key", key)

	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return RedeemKeyResult{}, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ws.HTTPClient().Do(req)
	if err != nil {
		return RedeemKeyResult{}, fmt.Errorf("请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RedeemKeyResult{}, fmt.Errorf("读取响应失败: %w", err)
	}

	// 响应 JSON 示例：{"success":1,"purchase_result_detail":0,"purchase_result_text":"..."} 或 {"success":0,...}
	var jsonObject map[string]any
	if err := json.Unmarshal(body, &jsonObject); err != nil || jsonObject == nil {
		return RedeemKeyResult{}, fmt.Errorf("响应解析失败: %s", string(body))
	}

	success := false
	if v, ok := jsonObject["success"].(float64); ok {
		success = v != 0
	}

	detail := ""
	if v, ok := jsonObject["purchase_result_text"].(string); ok {
		detail = v
	} else if v, ok := jsonObject["detail"].(string); ok {
		detail = v
	} else if v, ok := jsonObject["error"].(string); ok {
		detail = v
	}

	return RedeemKeyResult{Success: success, Detail: detail}, nil
}

// UnpackBooster 打开一个补充包（对应原版 ArchiWebHandler.UnpackBooster：
// POST https://steamcommunity.com/profiles/{steamID}/ajaxunpackbooster）。
func UnpackBooster(ws *steam.WebSession, steamID uint64, appID uint32, itemID uint64) (bool, error) {
	if ws == nil {
		return false, fmt.Errorf("web 会话为空")
	}
	if appID == 0 || itemID == 0 {
		return false, fmt.Errorf("appID/itemID 无效")
	}

	reqURL := fmt.Sprintf("https://steamcommunity.com/profiles/%d/ajaxunpackbooster", steamID)
	form := url.Values{}
	form.Set("appid", fmt.Sprintf("%d", appID))
	form.Set("communityitemid", fmt.Sprintf("%d", itemID))

	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ws.HTTPClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取响应失败: %w", err)
	}

	// 响应 JSON：{"success":1,"result":...} 或 {"success":0,...}
	var jsonObject map[string]any
	if err := json.Unmarshal(body, &jsonObject); err != nil || jsonObject == nil {
		return false, nil
	}
	if v, ok := jsonObject["success"].(float64); ok {
		return v != 0, nil
	}
	return false, nil
}

// AddFreeLicense 激活指定 subID 的免费许可（复刻原版 ArchiWebHandler.AddFreeLicense）。
// 需要已初始化的 Web 会话（sessionid + steamLoginSecure Cookie）。
func AddFreeLicense(ws *steam.WebSession, subID uint64) (AddFreeLicenseResult, error) {
	if ws == nil {
		return AddFreeLicenseResult{}, fmt.Errorf("web 会话为空")
	}
	if subID == 0 {
		return AddFreeLicenseResult{}, fmt.Errorf("subID 无效")
	}

	reqURL := fmt.Sprintf("%s/freelicense/addfreelicense/%d", SteamStoreURL, subID)
	form := url.Values{}
	form.Set("ajax", "true")

	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return AddFreeLicenseResult{}, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ws.HTTPClient().Do(req)
	if err != nil {
		return AddFreeLicenseResult{}, fmt.Errorf("请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AddFreeLicenseResult{}, fmt.Errorf("读取响应失败: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusNotFound, http.StatusServiceUnavailable:
		// Steam 例行维护
		return AddFreeLicenseResult{Result: EResultServiceUnavailable, PurchaseResultDetail: EPurchaseResultDetailTimeout}, nil
	case http.StatusForbidden:
		// 无权限
		return AddFreeLicenseResult{Result: EResultAccessDenied, PurchaseResultDetail: EPurchaseResultDetailInvalidPackage}, nil
	case http.StatusInternalServerError, http.StatusOK:
		// Steam 此接口的响应格式不稳定：可能是 []、{"purchaseresultdetail": N}、{"error": "..."} 或 null
		result := EResultFail
		if resp.StatusCode == http.StatusOK {
			result = EResultOK
		}

		var jsonObject map[string]any
		if err := json.Unmarshal(body, &jsonObject); err != nil || jsonObject == nil {
			// 响应不是 JSON 对象，无法解析更多细节
			return AddFreeLicenseResult{Result: result, PurchaseResultDetail: EPurchaseResultDetailNoDetail}, nil
		}

		if numberResult, ok := jsonObject["purchaseresultdetail"].(float64); ok {
			return AddFreeLicenseResult{Result: result, PurchaseResultDetail: PurchaseResultDetail(numberResult)}, nil
		}

		if errorMessage, ok := jsonObject["error"].(string); ok {
			switch {
			case strings.Contains(errorMessage, "already purchased"):
				return AddFreeLicenseResult{Result: result, PurchaseResultDetail: EPurchaseResultDetailAlreadyPurchased}, nil
			case strings.Contains(errorMessage, "invalid"):
				return AddFreeLicenseResult{Result: result, PurchaseResultDetail: EPurchaseResultDetailInvalidData}, nil
			}
		}

		return AddFreeLicenseResult{Result: result, PurchaseResultDetail: EPurchaseResultDetailNoDetail}, nil
	default:
		return AddFreeLicenseResult{Result: EResultFail, PurchaseResultDetail: EPurchaseResultDetailNoDetail}, nil
	}
}

// ECommunityPrivacy 对应 SteamKit2.ECommunityPrivacy。
const (
	ECommunityPrivacyInvalid     = 0
	ECommunityPrivacyPrivate     = 1
	ECommunityPrivacyFriendsOnly = 2
	ECommunityPrivacyPublic      = 3
)

// ECommentPermission 对应 SteamKit2.ECommentPermission。
const (
	ECommentPermissionFriendsOnly = 0
	ECommentPermissionAnyone      = 1
	ECommentPermissionSelfOnly    = 2
)

// PrivacySettings 对应原版 UserPrivacy.PrivacySettings 的 JSON 结构。
type PrivacySettings struct {
	PrivacyProfile        int `json:"PrivacyProfile"`
	PrivacyOwnedGames     int `json:"PrivacyOwnedGames"`
	PrivacyPlaytime       int `json:"PrivacyPlaytime"`
	PrivacyFriendsList    int `json:"PrivacyFriendsList"`
	PrivacyInventory      int `json:"PrivacyInventory"`
	PrivacyInventoryGifts int `json:"PrivacyInventoryGifts"`
}

// SetPrivacy 通过 Steam 社区 ajaxsetprivacy 接口修改个人资料隐私设置。
// ECommunityPrivacy: 1=私密 2=好友 3=公开；ECommentPermission: 0=好友 1=任何人 2=仅自己。
func SetPrivacy(ws *steam.WebSession, steamID uint64, settings PrivacySettings, commentPermission int) (bool, error) {
	if ws == nil {
		return false, fmt.Errorf("web 会话为空")
	}
	reqURL := fmt.Sprintf("https://steamcommunity.com/profiles/%d/ajaxsetprivacy", steamID)
	privacyJSON, err := json.Marshal(settings)
	if err != nil {
		return false, fmt.Errorf("序列化隐私设置失败: %w", err)
	}

	form := url.Values{}
	form.Set("sessionid", ws.SessionID())
	form.Set("eCommentPermission", strconv.Itoa(commentPermission))
	form.Set("Privacy", string(privacyJSON))

	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ws.HTTPClient().Do(req)
	if err != nil {
		return false, fmt.Errorf("请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("隐私设置返回状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取响应失败: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return false, nil
	}
	// 响应可能是 {"success":1} 或 {"result":1}
	if v, ok := result["success"].(float64); ok {
		return v != 0, nil
	}
	if v, ok := result["result"].(float64); ok {
		return v == 1, nil
	}
	return false, nil
}
