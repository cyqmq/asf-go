package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/asf-go/asf/internal/config"
)

func (b *Bot) checkAccess(chatterID uint64, min config.Access) bool {
	if containsUint64(b.Blacklist, chatterID) {
		return false
	}
	if b.OwnerSteamID != 0 && chatterID == b.OwnerSteamID {
		return true
	}
	userAccess, ok := b.Config.SteamUserPermissions[fmt.Sprintf("%d", chatterID)]
	if !ok {
		return false
	}
	return userAccess.Rank() >= min.Rank()
}

// handleCommand 处理 Steam 聊天命令（以 CommandPrefix 开头）。

func (b *Bot) handleCommand(chatterID uint64, message string) {
	reply := b.handleCommandText(chatterID, message)
	if b.steamClient != nil {
		b.steamClient.SendChatMessage(chatterID, reply)
	}
}

// handleCommandText 处理命令并返回回复文本（不发送，便于测试）。

func (b *Bot) handleCommandText(chatterID uint64, message string) string {
	prefix := b.CommandPrefix
	if prefix == "" {
		prefix = "!"
	}
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	command := strings.TrimSpace(strings.TrimPrefix(message, prefix))
	lower := strings.ToLower(command)

	b.Log.Info("收到来自 %d 的命令: %s", chatterID, command)

	var reply string

	switch {
	case lower == "help":
		reply = "可用命令: status, pause, resume, farm, restart, unpack, 2fa, mafile, password, keys, batch <key...>, fb <fbadd/fbrm>, fq <fqadd/fqrm>, loot <SteamID64>, transfer <BotName>, closetrades, nickname <name>, addlicense <sub/xxx>, checkappbeta <appid> <password>, owns <appid>, redeem <key>, level, points, stats, version, update, exit, help"
	case lower == "status":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		reply = b.statusText()
	case lower == "pause":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Pause()
		reply = "挂机已暂停"
	case lower == "resume":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Resume()
		reply = "挂机已恢复"
	case lower == "farm":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Resume()
		reply = "已重新开始挂机"
	case lower == "level":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		level, err := b.SteamLevel()
		if err != nil {
			reply = fmt.Sprintf("查询等级失败: %v", err)
		} else {
			reply = fmt.Sprintf("Steam 等级: %d", level)
		}
	case lower == "stats":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		reply = b.statsText()
	case lower == "version":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		if b.ASFVersion == "" {
			reply = "ASF Go 版"
		} else {
			reply = "ASF Go 版 " + b.ASFVersion
		}
	case lower == "points" || lower == "balance":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		points, err := b.PointsBalance()
		if err != nil {
			reply = fmt.Sprintf("查询积分失败: %v", err)
		} else {
			reply = fmt.Sprintf("Steam 积分: %d", points)
		}
	case lower == "restart":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		b.Pause()
		b.Resume()
		reply = "已重启挂机"
	case lower == "update":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		reply, _ = b.Update()
	case lower == "unpack":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		opened, err := b.UnpackBoosters()
		if err != nil {
			reply = fmt.Sprintf("开补充包失败: %v", err)
		} else {
			reply = fmt.Sprintf("已打开 %d 个补充包", opened)
		}
	case lower == "2fa":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		code, err := b.TwoFactorCode()
		if err != nil {
			reply = fmt.Sprintf("获取 2FA 代码失败: %v", err)
		} else {
			reply = "当前 2FA 代码: " + code
		}
	case lower == "mafile" || strings.HasPrefix(lower, "mafile "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		args := strings.Fields(command[len("mafile"):])
		if len(args) < 2 {
			if b.HasAuthenticator() {
				reply = "已配置移动令牌（device_id: " + b.Auth().DeviceID + "），用法: mafile <shared_secret> <identity_secret> [device_id]"
			} else {
				reply = "未配置移动令牌，用法: mafile <shared_secret> <identity_secret> [device_id]"
			}
			break
		}
		deviceID := ""
		if len(args) >= 3 {
			deviceID = args[2]
		}
		if err := b.ImportAuthenticator(args[0], args[1], deviceID); err != nil {
			reply = fmt.Sprintf("导入移动令牌失败: %v", err)
		} else {
			reply = "移动令牌已导入"
		}
	case lower == "password" || strings.HasPrefix(lower, "password "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		newPassword := strings.TrimSpace(command[len("password"):])
		if newPassword == "" {
			reply = "用法: password <新密码>"
			break
		}
		b.Config.SteamPassword = newPassword
		if err := b.saveConfig(); err != nil {
			reply = fmt.Sprintf("保存密码失败: %v", err)
		} else {
			reply = "Steam 密码已更新"
		}
	case lower == "keys":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		bgrPath := filepath.Join(b.configDir, b.Name+".games-to-redeem-in-background")
		data, err := os.ReadFile(bgrPath)
		if err != nil {
			reply = "后台兑换列表为空"
		} else {
			keysList := strings.Fields(string(data))
			if len(keysList) == 0 {
				reply = "后台兑换列表为空"
			} else {
				reply = "后台兑换密钥 (" + strconv.Itoa(len(keysList)) + "): " + strings.Join(keysList, ", ")
			}
		}
	case lower == "batch" || strings.HasPrefix(lower, "batch "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		keysList := strings.Fields(command[len("batch"):])
		if len(keysList) == 0 {
			reply = "用法: batch <CDKey...>"
			break
		}
		reply, _ = b.Redeem(keysList)
	case lower == "fb":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		if len(b.Config.FarmingBlacklist) == 0 {
			reply = "挂机黑名单为空"
		} else {
			ids := make([]string, 0, len(b.Config.FarmingBlacklist))
			for _, id := range b.Config.FarmingBlacklist {
				ids = append(ids, strconv.FormatUint(uint64(id), 10))
			}
			reply = "挂机黑名单: " + strings.Join(ids, ", ")
		}
	case strings.HasPrefix(lower, "fbadd "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fbadd"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingBlacklistAdd(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 加入挂机黑名单", appID)
			}
		}
	case strings.HasPrefix(lower, "fbrm "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fbrm"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingBlacklistRemove(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 移出挂机黑名单", appID)
			}
		}
	case lower == "fq":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		if len(b.Config.FarmingQueue) == 0 {
			reply = "挂机队列为空"
		} else {
			ids := make([]string, 0, len(b.Config.FarmingQueue))
			for _, id := range b.Config.FarmingQueue {
				ids = append(ids, strconv.FormatUint(uint64(id), 10))
			}
			reply = "挂机队列: " + strings.Join(ids, ", ")
		}
	case strings.HasPrefix(lower, "fqadd "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fqadd"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingQueueAdd(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 加入挂机队列", appID)
			}
		}
	case strings.HasPrefix(lower, "fqrm "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		appID, err := strconv.ParseUint(strings.TrimSpace(command[len("fqrm"):]), 10, 32)
		if err != nil {
			reply = "无效 appid"
		} else {
			if err := b.FarmingQueueRemove(uint32(appID)); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = fmt.Sprintf("已将 app/%d 移出挂机队列", appID)
			}
		}
	case lower == "exit":
		if b.OwnerSteamID == 0 || chatterID != b.OwnerSteamID {
			reply = "权限不足"
			break
		}
		reply = "正在退出 ASF"
		if b.OnExit != nil {
			b.OnExit()
		}
	case strings.HasPrefix(lower, "loot "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("loot"):])
		targetID, err := strconv.ParseUint(arg, 10, 64)
		if err != nil {
			reply = "无效 SteamID。用法: !loot <SteamID64>"
		} else {
			offerID, err := b.Loot(targetID)
			if err != nil {
				reply = fmt.Sprintf("发送报价失败: %v", err)
			} else {
				reply = fmt.Sprintf("已发送报价 %d", offerID)
			}
		}
	case strings.HasPrefix(lower, "transfer "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("transfer"):])
		targetID := uint64(0)
		if b.FindBotSteamID != nil {
			targetID = b.FindBotSteamID(arg)
		}
		if targetID == 0 {
			reply = fmt.Sprintf("目标 Bot %s 未连接或不存在", arg)
		} else {
			offerID, err := b.Loot(targetID)
			if err != nil {
				reply = fmt.Sprintf("发送报价失败: %v", err)
			} else {
				reply = fmt.Sprintf("已向 %s 发送报价 %d", arg, offerID)
			}
		}
	case lower == "closetrades":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		closed, err := b.CloseTrades()
		if err != nil {
			reply = fmt.Sprintf("关闭交易失败: %v", err)
		} else {
			reply = fmt.Sprintf("已关闭 %d 个交易报价", closed)
		}
	case strings.HasPrefix(lower, "nickname "):
		if !b.checkAccess(chatterID, config.AccessFamilySharing) {
			reply = "权限不足"
			break
		}
		nickname := strings.TrimSpace(command[len("nickname"):])
		if err := b.SetNickname(nickname); err != nil {
			reply = fmt.Sprintf("修改昵称失败: %v", err)
		} else {
			reply = fmt.Sprintf("昵称已修改为: %s", nickname)
		}
	case strings.HasPrefix(lower, "addlicense") || strings.HasPrefix(lower, "addlicence") || strings.HasPrefix(lower, "al"):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		arg := ""
		if lower != "addlicense" && lower != "addlicence" && lower != "al" {
			if idx := strings.Index(command, " "); idx >= 0 {
				arg = strings.TrimSpace(command[idx+1:])
			}
		}
		if arg == "" {
			reply = "用法: !addlicense <sub/xxx | app/xxx | 商店链接>（多个用空格分隔）"
		} else {
			identifiers := strings.Fields(arg)
			lines := make([]string, 0, len(identifiers))
			for _, ident := range identifiers {
				resp, err := b.AddLicense(ident)
				if err != nil {
					lines = append(lines, fmt.Sprintf("%s 失败: %v", ident, err))
				} else {
					lines = append(lines, resp)
				}
			}
			reply = strings.Join(lines, "; ")
		}
	case strings.HasPrefix(lower, "checkappbeta "):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		parts := strings.Fields(command[len("checkappbeta"):])
		if len(parts) < 2 {
			reply = "用法: !checkappbeta <appid> <password>"
		} else {
			appID, err := strconv.ParseUint(parts[0], 10, 32)
			if err != nil {
				reply = "无效 appid"
			} else {
				reply, err = b.CheckAppBeta(uint32(appID), parts[1])
				if err != nil {
					reply = fmt.Sprintf("beta 检查失败: %v", err)
				}
			}
		}
	case strings.HasPrefix(lower, "owns "):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("owns"):])
		if arg == "" {
			reply = "用法: !owns <appid> [appid...]"
		} else {
			resp, err := b.Owns(strings.Fields(arg))
			if err != nil {
				reply = fmt.Sprintf("查询失败: %v", err)
			} else {
				reply = resp
			}
		}
	case strings.HasPrefix(lower, "redeem "):
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("redeem"):])
		if arg == "" {
			reply = "用法: !redeem <激活码> [激活码...]"
		} else {
			resp, err := b.Redeem(strings.Fields(arg))
			if err != nil {
				reply = fmt.Sprintf("兑换失败: %v", err)
			} else {
				reply = resp
			}
		}
	default:
		reply = "未知命令。发送 !help 查看帮助。"
	}

	return reply
}

// statusText 生成 Bot 状态文本。

func (b *Bot) statusText() string {
	state := "未连接"
	if b.IsConnected() {
		state = "已登录"
	}
	text := fmt.Sprintf("%s | %s | 在线状态: %s", b.Name, state, b.Config.OnlineStatus)
	games := b.FarmingGames()
	if len(games) == 0 {
		text += " | 当前无挂机游戏"
	} else {
		text += fmt.Sprintf(" | 挂机游戏 %d 个", len(games))
		for _, g := range games {
			text += fmt.Sprintf(" | %s(%d):%d", g.Name, g.AppID, g.CardsRemaining)
		}
	}
	return text
}

// IsFarming 返回卡牌挂机是否正在运行。

func (b *Bot) statsText() string {
	state := "未连接"
	if b.IsConnected() {
		state = "已登录"
	}
	text := fmt.Sprintf("%s | %s | 在线状态: %s", b.Name, state, b.Config.OnlineStatus)
	games := b.FarmingGames()
	if len(games) == 0 {
		text += " | 当前无挂机游戏"
	} else {
		text += fmt.Sprintf(" | 挂机游戏 %d 个", len(games))
		for _, g := range games {
			text += fmt.Sprintf(" | %s(%d):%d", g.Name, g.AppID, g.CardsRemaining)
		}
	}
	text += fmt.Sprintf(" | 交易模块: %v | 好友请求自动接受: %v", b.Config.Trading, b.Config.AcceptFriendRequests)
	return text
}

// String 实现 fmt.Stringer。
