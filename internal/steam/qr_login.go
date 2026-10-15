// QR 登录协议（Steam Web API 认证），对应 SteamKit2 的 QrAuthSession。
// 使用公开的 IAuthenticationService 端点：
//   - BeginAuthSessionViaQR/v1：创建 QR 会话，返回 challenge_url
//   - PollAuthSessionStatus/v1：轮询扫码状态，完成后返回 refresh_token/access_token
package steam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// qrAuthBaseURL 是 Steam 认证 Web API 端点。
const qrAuthBaseURL = "https://api.steampowered.com/IAuthenticationService"

// QRLoginSession 是一次 QR 登录会话。
type QRLoginSession struct {
	ClientID     string
	RequestID    string
	ChallengeURL string
	Interval     int

	RefreshToken string
	AccessToken  string
	AccountName  string
	SteamID      uint64

	httpClient *http.Client
}

// StartQRLogin 发起一次 QR 登录会话，返回会话与 challenge URL。
// challenge URL 可直接生成二维码（steam:// 或 https://s.team/q/...）。
func StartQRLogin(httpClient *http.Client) (*QRLoginSession, string, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	requestBody := map[string]any{
		"device_details": map[string]any{
			"device_friendly_name": "ASF",
			"platform_type":        2, // EAuthTokenPlatformType_WebBrowser
			"os_type":              -1,
		},
		"website_id": "Community",
	}

	req, err := http.NewRequest(http.MethodPost, qrAuthBaseURL+"/BeginAuthSessionViaQR/v1", jsonReader(requestBody))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("BeginAuthSessionViaQR 返回 %d: %s", resp.StatusCode, string(body))
	}

	var envelope struct {
		Response struct {
			ClientID     string `json:"client_id"`
			RequestID    string `json:"request_id"`
			ChallengeURL string `json:"challenge_url"`
			Interval     int    `json:"interval"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, "", err
	}

	session := &QRLoginSession{
		ClientID:     envelope.Response.ClientID,
		RequestID:    envelope.Response.RequestID,
		ChallengeURL: envelope.Response.ChallengeURL,
		Interval:     envelope.Response.Interval,
		httpClient:   httpClient,
	}
	if session.Interval <= 0 {
		session.Interval = 5
	}

	return session, session.ChallengeURL, nil
}

// Poll 轮询一次扫码状态。
// 返回 true 表示扫码确认完成（RefreshToken/AccessToken 已填充）。
func (s *QRLoginSession) Poll() (bool, error) {
	if s.ClientID == "" || s.RequestID == "" {
		return false, fmt.Errorf("QR 会话未初始化")
	}

	inputJSON, err := json.Marshal(map[string]any{
		"client_id":  s.ClientID,
		"request_id": s.RequestID,
	})
	if err != nil {
		return false, err
	}

	form := url.Values{}
	form.Set("input_json", string(inputJSON))

	req, err := http.NewRequest(http.MethodPost, qrAuthBaseURL+"/PollAuthSessionStatus/v1", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("PollAuthSessionStatus 返回 %d: %s", resp.StatusCode, string(body))
	}

	var envelope struct {
		Response struct {
			NewChallengeURL      string `json:"new_challenge_url"`
			RefreshToken         string `json:"refresh_token"`
			AccessToken          string `json:"access_token"`
			AccountName          string `json:"account_name"`
			SteamID              string `json:"steamid"`
			PollInterval         int    `json:"poll_interval"`
			HadRemoteInteraction bool   `json:"had_remote_interaction"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return false, err
	}

	// Steam 会周期性刷新挑战 URL（二维码有效期约 60 秒），需要同步更新。
	if envelope.Response.NewChallengeURL != "" {
		s.ChallengeURL = envelope.Response.NewChallengeURL
	}
	if envelope.Response.PollInterval > 0 {
		s.Interval = envelope.Response.PollInterval
	}

	s.AccountName = envelope.Response.AccountName
	s.RefreshToken = envelope.Response.RefreshToken
	s.AccessToken = envelope.Response.AccessToken
	if envelope.Response.SteamID != "" {
		if steamID, err := strconv.ParseUint(envelope.Response.SteamID, 10, 64); err == nil {
			s.SteamID = steamID
		}
	}

	return s.RefreshToken != "" || s.AccessToken != "", nil
}

// LoginToken 返回用于 CM 登录的令牌（优先 refresh token，与 SteamKit2 行为一致）。
func (s *QRLoginSession) LoginToken() string {
	if s.RefreshToken != "" {
		return s.RefreshToken
	}
	return s.AccessToken
}

func jsonReader(v any) io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		return bytes.NewReader(nil)
	}
	return bytes.NewReader(data)
}
