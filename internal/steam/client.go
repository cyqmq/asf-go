// Package steam 封装 go-steam 协议库，对应 ASF 的 Steam/SteamKit2 层。
package steam

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asf-go/asf/internal/logging"
	"github.com/cyqmq/go-steam/v3"
	"github.com/cyqmq/go-steam/v3/protocol/steamlang"
	"github.com/cyqmq/go-steam/v3/steamid"
)

// Client 是对 go-steam Client 的薄封装。
type Client struct {
	inner  *steam.Client
	log    *logging.Logger
	events chan any

	username string
	password string
	loginKey string
	authCode string
	twoFA    string

	onLoginKey      func(string)
	onChatMessage   func(chatterID uint64, message string)
	onFriendRequest func(friendID uint64)
	onChatInvite    func(inviterID uint64, chatRoomID uint64, roomName string)

	webSession *WebSession

	loggedOn   chan struct{}
	isLoggedOn atomic.Bool
	stopCh     chan struct{}
}

// NewClient 创建 Steam 客户端。
func NewClient(log *logging.Logger, username, password, loginKey string) *Client {
	c := &Client{
		inner:    steam.NewClient(),
		log:      log,
		events:   make(chan any, 64),
		username: username,
		password: password,
		loginKey: loginKey,
		loggedOn: make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
	return c
}

// SetOnLoginKey 注册 LoginKey 变更回调（用于持久化）。
func (c *Client) SetOnLoginKey(fn func(string)) {
	c.onLoginKey = fn
}

// SetOnChatMessage 注册聊天消息回调（用于命令处理）。
func (c *Client) SetOnChatMessage(fn func(chatterID uint64, message string)) {
	c.onChatMessage = fn
}

// SetOnFriendRequest 注册好友请求回调（收到新的好友请求时触发）。
func (c *Client) SetOnFriendRequest(fn func(friendID uint64)) {
	c.onFriendRequest = fn
}

// SetOnChatInvite 注册聊天/群组邀请回调。
func (c *Client) SetOnChatInvite(fn func(inviterID uint64, chatRoomID uint64, roomName string)) {
	c.onChatInvite = fn
}

// SetAuthCode 设置 Steam Guard 邮箱验证码。
func (c *Client) SetAuthCode(code string) {
	c.authCode = code
}

// SetTwoFactorCode 设置 Steam Guard 手机令牌。
func (c *Client) SetTwoFactorCode(code string) {
	c.twoFA = code
}

// Connect 连接 Steam 网络并等待登录完成（带超时）。
func (c *Client) Connect(timeout time.Duration) error {
	c.inner.Connect()
	go c.eventLoop()

	select {
	case <-c.loggedOn:
		c.log.Info("Steam 登录成功 (%s)", c.username)
		return nil
	case <-time.After(timeout):
		return errors.New("Steam 登录超时")
	}
}

func (c *Client) eventLoop() {
	for {
		select {
		case <-c.stopCh:
			return
		case e := <-c.inner.Events():
			switch ev := e.(type) {
			case *steam.ConnectedEvent:
				c.log.Debug("已连接到 Steam 网络，开始登录")
				details := &steam.LogOnDetails{
					Username:               c.username,
					ShouldRememberPassword: true,
				}
				if c.password != "" {
					details.Password = c.password
				}
				if c.loginKey != "" {
					details.LoginKey = c.loginKey
				}
				if c.authCode != "" {
					details.AuthCode = c.authCode
				}
				if c.twoFA != "" {
					details.TwoFactorCode = c.twoFA
				}
				c.inner.Auth.LogOn(details)

			case *steam.LoggedOnEvent:
				c.log.Debug("已登录，设置在线状态")
				c.isLoggedOn.Store(true)
				c.inner.Social.SetPersonaState(steamlang.EPersonaState_Online)
				close(c.loggedOn)

			case *steam.LoginKeyEvent:
				c.log.Debug("收到新 LoginKey")
				c.loginKey = ev.LoginKey
				if c.onLoginKey != nil {
					c.onLoginKey(ev.LoginKey)
				}

			case *steam.MachineAuthUpdateEvent:
				c.log.Debug("收到 MachineAuth 更新")

			case *steam.ChatMsgEvent:
				if ev.IsMessage() && c.onChatMessage != nil {
					c.onChatMessage(uint64(ev.ChatterId), ev.Message)
				}

			case *steam.FriendStateEvent:
				// 收到好友请求（等待我方确认）
				if ev.Relationship == steamlang.EFriendRelationship_RequestRecipient && c.onFriendRequest != nil {
					c.log.Debug("收到好友请求: %d", uint64(ev.SteamId))
					c.onFriendRequest(uint64(ev.SteamId))
				}

			case *steam.ChatInviteEvent:
				if c.onChatInvite != nil {
					c.log.Debug("收到聊天邀请: %s (来自 %d)", ev.ChatRoomName, uint64(ev.PatronId))
					c.onChatInvite(uint64(ev.PatronId), uint64(ev.ChatRoomId), ev.ChatRoomName)
				}

			case *steam.DisconnectedEvent:
				c.log.Warn("与 Steam 断开连接")

			case steam.FatalErrorEvent:
				c.log.Error("Steam 致命错误: %v", ev)

			case error:
				c.log.Error("Steam 错误: %v", ev)
			}
		}
	}
}

// Disconnect 断开 Steam 连接。
func (c *Client) Disconnect() {
	close(c.stopCh)
	c.inner.Disconnect()
}

// InitWebSession 初始化 steamcommunity Web 会话（用于抓取卡牌页面）。
func (c *Client) InitWebSession() error {
	// 等待 Web sessionid 就绪
	deadline := time.Now().Add(10 * time.Second)
	for c.inner.Web.SessionId == "" {
		if time.Now().After(deadline) {
			return errors.New("等待 Web sessionid 超时")
		}
		time.Sleep(200 * time.Millisecond)
	}

	c.inner.Web.LogOn()

	// 等待 steamLoginSecure Cookie 生成
	deadline = time.Now().Add(15 * time.Second)
	for c.inner.Web.SteamLoginSecure == "" {
		if time.Now().After(deadline) {
			return errors.New("等待 steamLoginSecure 超时")
		}
		time.Sleep(200 * time.Millisecond)
	}

	ws, err := NewWebSession(c.inner.Web.SessionId, c.inner.Web.SteamLoginSecure)
	if err != nil {
		return err
	}
	c.webSession = ws
	c.log.Debug("Web 会话已就绪")
	return nil
}

// WebSession 返回已初始化的 Web 会话。
func (c *Client) WebSession() (*WebSession, error) {
	if c.webSession == nil {
		return nil, errors.New("Web 会话尚未初始化")
	}
	return c.webSession, nil
}

// WebSessionOrNil 返回已初始化的 Web 会话；未初始化时返回 nil（供交易模块容错使用）。
func (c *Client) WebSessionOrNil() *WebSession {
	return c.webSession
}

// StartPlaying 设置游玩中的游戏（卡牌挂机）。
func (c *Client) StartPlaying(appIDs []uint64) {
	c.inner.GC.SetGamesPlayed(appIDs...)
	c.log.Debug("设置游玩游戏: %v", appIDs)
}

// StopPlaying 清除游玩状态。
func (c *Client) StopPlaying() {
	c.inner.GC.SetGamesPlayed()
	c.log.Debug("已停止所有游玩游戏")
}

// SendChatMessage 向指定 SteamID 发送聊天消息。
func (c *Client) SendChatMessage(to uint64, message string) {
	c.inner.Social.SendMessage(steamid.SteamId(to), steamlang.EChatEntryType_ChatMsg, message)
	c.log.Debug("向 %d 发送消息: %s", to, message)
}

// AcceptFriend 接受好友请求。
func (c *Client) AcceptFriend(friendID uint64) {
	c.inner.Social.AddFriend(steamid.SteamId(friendID))
	c.log.Debug("接受好友请求: %d", friendID)
}

// RemoveFriend 删除或拒绝好友。
func (c *Client) RemoveFriend(friendID uint64) {
	c.inner.Social.RemoveFriend(steamid.SteamId(friendID))
	c.log.Debug("删除好友: %d", friendID)
}

// JoinChat 加入聊天房间。
func (c *Client) JoinChat(chatRoomID uint64) {
	c.inner.Social.JoinChat(steamid.SteamId(chatRoomID))
	c.log.Debug("加入聊天: %d", chatRoomID)
}

// RequestFreeLicense 请求指定 appID 的免费许可（通过 CM 协议，对应原版 SteamApps.RequestFreeLicense）。
func (c *Client) RequestFreeLicense(appID uint32) (*steam.FreeLicenseResult, error) {
	return c.inner.Apps.RequestFreeLicense(appID)
}

// CheckAppBetaPassword 提交指定 appID 的 beta 密码，返回可用 beta 分支及其加密密钥。
func (c *Client) CheckAppBetaPassword(appID uint32, password string) (*steam.CheckAppBetaPasswordResult, error) {
	return c.inner.Apps.CheckAppBetaPassword(appID, password)
}

// LoginKey 返回最近一次登录获得的 LoginKey。
func (c *Client) LoginKey() string {
	return c.loginKey
}

// LoggedOn 返回是否已成功登录 Steam。
func (c *Client) LoggedOn() bool {
	return c.isLoggedOn.Load()
}

// Connected 返回是否已连接 Steam 网络。
func (c *Client) Connected() bool {
	return c.inner.Connected()
}

// SteamID 返回当前登录账号的 SteamID（未登录时为 0）。
func (c *Client) SteamID() uint64 {
	return uint64(c.inner.SteamId())
}

// String 实现 fmt.Stringer。
func (c *Client) String() string {
	return fmt.Sprintf("SteamClient(%s)", c.username)
}
