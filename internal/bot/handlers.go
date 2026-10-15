package bot

import (
	"fmt"

	"github.com/asf-go/asf/internal/config"
)

func (b *Bot) SetNickname(name string) error {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if name == "" {
		return fmt.Errorf("昵称不能为空")
	}
	b.steamClient.SetPersonaName(name)
	return nil
}

// handleFriendRequest 处理收到的好友请求。

func (b *Bot) handleFriendRequest(friendID uint64) {
	b.Log.Info("收到好友请求: %d", friendID)

	switch {
	case b.shouldAcceptFriend(friendID):
		b.Log.Info("接受好友请求: %d", friendID)
		b.steamClient.AcceptFriend(friendID)
		b.notifyOwner(fmt.Sprintf("Bot %s 接受了 %d 的好友请求", b.Name, friendID))
	case b.Config.BotBehaviour.Has(config.BotBehaviourRejectInvalidFriendRequests):
		b.Log.Info("拒绝无效好友请求: %d", friendID)
		b.steamClient.RemoveFriend(friendID)
		b.notifyOwner(fmt.Sprintf("Bot %s 拒绝了无效好友请求: %d", b.Name, friendID))
	default:
		b.Log.Info("忽略好友请求: %d（未配置自动接受或拒绝）", friendID)
	}
}

// shouldAcceptFriend 判断是否应接受好友请求。

func (b *Bot) shouldAcceptFriend(friendID uint64) bool {
	if b.Config.AcceptFriendRequests {
		return true
	}
	if _, ok := b.Config.SteamUserPermissions[fmt.Sprintf("%d", friendID)]; ok {
		return true
	}
	if containsUint64(b.Config.TradingAllowableSteamIDs, friendID) {
		return true
	}
	return false
}

// handleChatInvite 处理聊天/群组邀请。

func (b *Bot) handleChatInvite(inviterID, chatRoomID uint64, roomName string) {
	b.Log.Info("收到聊天邀请: %q (来自 %d)", roomName, inviterID)

	if b.Config.BotBehaviour.Has(config.BotBehaviourRejectInvalidGroupInvites) && !b.shouldAcceptFriend(inviterID) {
		b.Log.Info("拒绝无效群组邀请: %q", roomName)
		b.notifyOwner(fmt.Sprintf("Bot %s 拒绝了群组邀请: %s", b.Name, roomName))
		return
	}
	b.Log.Info("接受聊天邀请并加入: %q", roomName)
	b.steamClient.JoinChat(chatRoomID)
}

// notifyOwner 向 SteamOwnerID 发送通知消息（若 owner 为好友则成功，否则仅记录日志）。

func (b *Bot) notifyOwner(message string) {
	if b.OwnerSteamID == 0 {
		return
	}
	b.Log.Info("通知 Owner: %s", message)
	if b.steamClient != nil && b.steamClient.LoggedOn() {
		b.steamClient.SendChatMessage(b.OwnerSteamID, message)
	}
}

// Loot 将库存中的可转移物品（卡片/闪卡/补充包）发送给指定用户，返回报价 ID。
