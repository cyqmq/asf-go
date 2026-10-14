// Package steam 的 Web 会话：使用 go-steam 登录后的 Cookie 访问 steamcommunity.com。
package steam

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// SteamCommunityURL 是社区站点根地址。
const SteamCommunityURL = "https://steamcommunity.com"

// SteamStoreURL 是 Steam 商店根地址。
const SteamStoreURL = "https://store.steampowered.com"

// WebSession 携带登录会话 Cookie，用于抓取卡牌页面等。
type WebSession struct {
	client           *http.Client
	sessionID        string
	steamLoginSecure string
}

// NewWebSession 基于 go-steam Web 模块提供的会话凭证创建。
func NewWebSession(sessionID, steamLoginSecure string) (*WebSession, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("创建 cookie jar 失败: %w", err)
	}
	cookies := []*http.Cookie{
		{Name: "sessionid", Value: sessionID},
		{Name: "steamLoginSecure", Value: steamLoginSecure},
	}
	for _, rawURL := range []string{SteamCommunityURL, SteamStoreURL} {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		jar.SetCookies(u, cookies)
	}
	return &WebSession{client: &http.Client{Jar: jar}, sessionID: sessionID, steamLoginSecure: steamLoginSecure}, nil
}

// SessionID 返回 steamcommunity 会话 ID。
func (w *WebSession) SessionID() string {
	return w.sessionID
}

// SteamLoginSecure 返回 steamLoginSecure Cookie 值。
func (w *WebSession) SteamLoginSecure() string {
	return w.steamLoginSecure
}

// HTTPClient 返回带登录 Cookie 的 HTTP 客户端，供其他模块（如库存、交易）复用。
func (w *WebSession) HTTPClient() *http.Client {
	return w.client
}

// GetBadgePage 抓取指定页的卡牌页面，返回 goquery 文档。
func (w *WebSession) GetBadgePage(page uint8) (*goquery.Document, error) {
	reqURL := fmt.Sprintf("%s/my/badges?l=english&p=%d", SteamCommunityURL, page)
	resp, err := w.client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("请求卡牌页面失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("卡牌页面返回状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取卡牌页面失败: %w", err)
	}
	if strings.Contains(string(body), "There was an error getting your badge data") {
		return nil, errors.New("卡牌页面包含错误提示")
	}
	return goquery.NewDocumentFromReader(strings.NewReader(string(body)))
}
