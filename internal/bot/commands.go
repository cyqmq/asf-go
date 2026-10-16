package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/asf-go/asf/internal/config"
	asfcrypto "github.com/asf-go/asf/internal/crypto"
	"github.com/asf-go/asf/internal/store"
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
// chatRoomID 为 0 表示私聊，非零表示群聊（群聊需要 bot 名或 !all 寻址）。

func (b *Bot) handleCommand(chatterID uint64, message string, chatRoomID uint64) {
	if !b.remoteCommunicationAllowsSteam() {
		return
	}
	if chatRoomID != 0 {
		command := stripGroupTarget(message, b.CommandPrefix, b.Name)
		if command == "" {
			return
		}
		message = command
	}
	reply := b.handleCommandText(chatterID, message)
	if b.steamClient != nil {
		if prefix := b.messagePrefix(); prefix != "" {
			reply = prefix + reply
		}
		if chatRoomID != 0 {
			b.steamClient.SendChatMessageToRoom(chatRoomID, reply)
		} else {
			b.steamClient.SendChatMessage(chatterID, reply)
		}
	}
}

// stripGroupTarget 解析群聊命令寻址：消息形如 "!<botname> <command>" 或 "!<all> <command>"。
// 目标匹配时返回去掉寻址前缀后的命令（保持原前缀），否则返回空字符串。
func stripGroupTarget(message, prefix, botName string) string {
	if prefix == "" {
		prefix = "!"
	}
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(message, prefix))
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return ""
	}
	target := strings.ToLower(fields[0])
	if target != "all" && target != strings.ToLower(botName) {
		return ""
	}
	return prefix + strings.Join(fields[1:], " ")
}

// messagePrefix 返回 Steam 消息前缀（GlobalConfig.SteamMessagePrefix，如 "/me "）。
func (b *Bot) messagePrefix() string {
	if b.GlobalConfig != nil {
		return b.GlobalConfig.SteamMessagePrefix
	}
	return ""
}

// remoteCommunicationAllowsSteam 根据 RemoteCommunication 配置决定是否响应 Steam 聊天命令。
// 支持 "All"/"SteamGroup"/"PublicListing"/"None" 或数字位标志（SteamGroup=1）。
func (b *Bot) remoteCommunicationAllowsSteam() bool {
	value := strings.ToLower(strings.TrimSpace(string(b.Config.RemoteCommunication)))
	switch value {
	case "", "all":
		return true
	case "none":
		return false
	case "publiclisting":
		return false
	case "steamgroup":
		return true
	default:
		if n, err := strconv.ParseUint(value, 10, 8); err == nil {
			return n&1 != 0
		}
		return true
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
	case lower == "help" || strings.HasPrefix(lower, "help "):
		arg := strings.TrimSpace(command[len("help"):])
		if arg != "" {
			if usage, ok := commandHelp[strings.ToLower(arg)]; ok {
				reply = fmt.Sprintf("%s: %s", arg, usage)
			} else {
				reply = fmt.Sprintf("未知命令 %q。发送 !help 查看全部命令。", arg)
			}
			break
		}
		reply = "可用命令: status, pause, resume, farm, restart, unpack, 2fa, 2faok, 2fano, mafile, password, keys, batch, inventory, play, input, privacy, bgr, bgrm, bgrclear, hash, encrypt, reset, mab, mabadd, mabrm, fb, fbadd, fbrm, fq, fqadd, fqrm, loot, transfer, closetrades, nickname, addlicense, checkappbeta, owns, redeem, level, points, stats, version, update, exit, help\n发送 !help <命令> 查看单个命令用法。"
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
		b.Config.SetSteamPassword(newPassword)
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
			// 无参数：从后台兑换列表加载并批量兑换
			bgrFile := filepath.Join(b.configDir, b.Name+".games-to-redeem-in-background")
			data, err := os.ReadFile(bgrFile)
			if err != nil {
				reply = "后台兑换列表为空。用法: batch <CDKey...>，或先用 !bgr <key> 添加"
				break
			}
			keysList = strings.Fields(string(data))
			if len(keysList) == 0 {
				reply = "后台兑换列表为空。用法: batch <CDKey...>，或先用 !bgr <key> 添加"
				break
			}
			resp, err := b.Redeem(keysList)
			if err != nil {
				reply = fmt.Sprintf("批量兑换失败: %v（后台列表已保留）", err)
				break
			}
			// 兑换成功后清空后台列表
			_ = os.WriteFile(bgrFile, nil, 0o600)
			reply = resp + fmt.Sprintf("（已从后台兑换列表加载 %d 个 key 并清空）", len(keysList))
			break
		}
		reply, _ = b.Redeem(keysList)
	case lower == "inventory":
		if !b.checkAccess(chatterID, config.AccessOperator) {
			reply = "权限不足"
			break
		}
		inv, err := b.GetInventory(753, 6)
		if err != nil {
			reply = fmt.Sprintf("查询库存失败: %v", err)
			break
		}
		if len(inv.Items) == 0 {
			reply = "库存为空"
			break
		}
		counts := make(map[string]uint64)
		for _, item := range inv.Items {
			counts[item.Name] += item.Amount
		}
		names := make([]string, 0, len(counts))
		for name := range counts {
			names = append(names, name)
		}
		sort.Strings(names)
		lines := make([]string, 0, len(names))
		for _, name := range names {
			lines = append(lines, fmt.Sprintf("%s x%d", name, counts[name]))
		}
		reply = "库存 (753/6):\n" + strings.Join(lines, "\n")
	case lower == "play" || strings.HasPrefix(lower, "play "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("play"):])
		if arg == "" {
			b.Resume()
			reply = "已恢复自动挂机"
			break
		}
		var appIDs []uint64
		valid := true
		for _, f := range strings.Fields(arg) {
			id, err := strconv.ParseUint(f, 10, 32)
			if err != nil {
				reply = "无效 appid: " + f
				valid = false
				break
			}
			appIDs = append(appIDs, id)
		}
		if valid {
			if b.steamClient != nil {
				b.steamClient.StartPlaying(appIDs)
				reply = fmt.Sprintf("正在游玩 %d 个游戏", len(appIDs))
			} else {
				reply = "Steam 客户端未就绪"
			}
		}
	case lower == "2faok" || lower == "2fano":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		confirmations, err := b.GetConfirmations()
		if err != nil {
			reply = fmt.Sprintf("获取确认失败: %v", err)
			break
		}
		if len(confirmations) == 0 {
			reply = "没有待确认项"
			break
		}
		accept := lower == "2faok"
		ok, err := b.HandleConfirmations(accept, confirmations)
		if err != nil {
			reply = fmt.Sprintf("处理确认失败: %v", err)
			break
		}
		if accept {
			reply = fmt.Sprintf("已接受 %d 个确认", len(confirmations))
		} else {
			reply = fmt.Sprintf("已拒绝 %d 个确认", len(confirmations))
		}
		if !ok {
			reply += "（部分操作未成功）"
		}
	case lower == "input" || strings.HasPrefix(lower, "input "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		code := strings.TrimSpace(command[len("input"):])
		if code == "" {
			reply = "用法: !input <Steam Guard/2FA 验证码>"
			break
		}
		if err := b.SubmitInput(code); err != nil {
			reply = fmt.Sprintf("提交验证码失败: %v", err)
		} else {
			reply = "验证码已设置，正在重新登录..."
		}
	case lower == "privacy" || strings.HasPrefix(lower, "privacy "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		args := strings.Fields(command[len("privacy"):])
		if len(args) == 0 {
			reply = "用法: !privacy <profile> [ownedGames] [playtime] [friendsList] [inventory] [inventoryGifts] [comments]；隐私值 1=私密 2=好友 3=公开，comments 0=好友 1=任何人 2=仅自己"
			break
		}
		if len(args) > 7 {
			reply = "参数过多，最多 7 个"
			break
		}
		values := []int{store.ECommunityPrivacyPrivate, store.ECommunityPrivacyPrivate, store.ECommunityPrivacyPrivate, store.ECommunityPrivacyPrivate, store.ECommunityPrivacyPrivate, store.ECommunityPrivacyPrivate, store.ECommentPermissionSelfOnly}
		valid := true
		for i, arg := range args {
			v, err := strconv.Atoi(arg)
			if err != nil || v < 0 || v > 3 {
				reply = fmt.Sprintf("第 %d 个参数 %q 无效，隐私值应为 1/2/3（comments 为 0/1/2）", i+1, arg)
				valid = false
				break
			}
			values[i] = v
		}
		if !valid {
			break
		}
		settings := store.PrivacySettings{
			PrivacyProfile:        values[0],
			PrivacyOwnedGames:     values[1],
			PrivacyPlaytime:       values[2],
			PrivacyFriendsList:    values[3],
			PrivacyInventory:      values[4],
			PrivacyInventoryGifts: values[5],
		}
		ok, err := b.SetPrivacy(settings, values[6])
		if err != nil {
			reply = fmt.Sprintf("修改隐私失败: %v", err)
			break
		}
		if ok {
			reply = "隐私设置已更新"
		} else {
			reply = "修改隐私失败（Steam 返回错误）"
		}
	case lower == "bgr" || strings.HasPrefix(lower, "bgr "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		keyArg := strings.TrimSpace(command[len("bgr"):])
		if keyArg == "" {
			reply = "用法: !bgr <key>，或 !bgrc/!bgrclear 清空、!bgrm <key> 移除"
			break
		}
		bgrFile := filepath.Join(b.configDir, b.Name+".games-to-redeem-in-background")
		keys := []string{}
		if data, err := os.ReadFile(bgrFile); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					keys = append(keys, line)
				}
			}
		}
		if containsString(keys, keyArg) {
			reply = "该 key 已在后台兑换列表中"
			break
		}
		keys = append(keys, keyArg)
		sort.Strings(keys)
		if err := os.WriteFile(bgrFile, []byte(strings.Join(keys, "\n")+"\n"), 0o600); err != nil {
			reply = fmt.Sprintf("写入后台兑换列表失败: %v", err)
			break
		}
		reply = fmt.Sprintf("已添加 key 到后台兑换列表，共 %d 个", len(keys))
	case lower == "bgrc" || lower == "bgrclear":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		bgrFile := filepath.Join(b.configDir, b.Name+".games-to-redeem-in-background")
		if err := os.WriteFile(bgrFile, nil, 0o600); err != nil {
			reply = fmt.Sprintf("清空后台兑换列表失败: %v", err)
			break
		}
		reply = "后台兑换列表已清空"
	case lower == "bgrm" || strings.HasPrefix(lower, "bgrm "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		keyArg := strings.TrimSpace(command[len("bgrm"):])
		if keyArg == "" {
			reply = "用法: !bgrm <key>"
			break
		}
		bgrFile := filepath.Join(b.configDir, b.Name+".games-to-redeem-in-background")
		keys := []string{}
		if data, err := os.ReadFile(bgrFile); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					keys = append(keys, line)
				}
			}
		}
		remaining := keys[:0]
		found := false
		for _, k := range keys {
			if k == keyArg {
				found = true
				continue
			}
			remaining = append(remaining, k)
		}
		if !found {
			reply = "后台兑换列表中不存在该 key"
			break
		}
		sort.Strings(remaining)
		if err := os.WriteFile(bgrFile, []byte(strings.Join(remaining, "\n")+"\n"), 0o600); err != nil {
			reply = fmt.Sprintf("写入后台兑换列表失败: %v", err)
			break
		}
		reply = fmt.Sprintf("已从后台兑换列表移除 key，剩余 %d 个", len(remaining))
	case lower == "hash" || strings.HasPrefix(lower, "hash "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		text := strings.TrimSpace(command[len("hash"):])
		if text == "" {
			reply = "用法: !hash <text>，返回 SCrypt 哈希（IPC 密码格式）"
			break
		}
		hash, err := asfcrypto.HashSCrypt(text)
		if err != nil {
			reply = fmt.Sprintf("哈希计算失败: %v", err)
			break
		}
		reply = fmt.Sprintf("SCrypt 哈希: %s", hash)
	case lower == "encrypt" || strings.HasPrefix(lower, "encrypt "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		text := strings.TrimSpace(command[len("encrypt"):])
		if text == "" {
			reply = "用法: !encrypt <text>，返回 ASF 加密后的密码"
			break
		}
		encrypted, err := asfcrypto.EncryptAES(text)
		if err != nil {
			reply = fmt.Sprintf("加密失败: %v", err)
			break
		}
		reply = fmt.Sprintf("加密结果: %s", encrypted)
	case lower == "reset" || lower == "rp":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		if err := os.Remove(b.loginKeyPath()); err != nil && !os.IsNotExist(err) {
			reply = fmt.Sprintf("删除登录 Key 失败: %v", err)
			break
		}
		b.loginKey = ""
		reply = "已重置登录 Key，正在重新登录..."
		b.stopModules()
		go b.scheduleReconnect()
	case lower == "mab":
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		reply = b.formatMAB()
	case lower == "mabadd" || strings.HasPrefix(lower, "mabadd "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("mabadd"):])
		if arg == "" {
			reply = "用法: !mabadd <appid[,appid...]>"
			break
		}
		appIDs, err := parseAppIDs(arg)
		if err != nil {
			reply = fmt.Sprintf("无效 appid: %v", err)
			break
		}
		if err := b.MatchActivelyBlacklistAdd(appIDs); err != nil {
			reply = fmt.Sprintf("保存失败: %v", err)
		} else {
			reply = fmt.Sprintf("已将 %d 个游戏加入主动匹配黑名单", len(appIDs))
		}
	case lower == "mabrm" || strings.HasPrefix(lower, "mabrm "):
		if !b.checkAccess(chatterID, config.AccessMaster) {
			reply = "权限不足"
			break
		}
		arg := strings.TrimSpace(command[len("mabrm"):])
		if arg == "" {
			reply = "用法: !mabrm <appid[,appid...]|all>"
			break
		}
		if strings.EqualFold(arg, "all") {
			if err := b.MatchActivelyBlacklistRemove(nil); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = "已清空主动匹配黑名单"
			}
			break
		}
		appIDs, err := parseAppIDs(arg)
		if err != nil {
			reply = fmt.Sprintf("无效 appid: %v", err)
			break
		}
		if err := b.MatchActivelyBlacklistRemove(appIDs); err != nil {
			reply = fmt.Sprintf("保存失败: %v", err)
		} else {
			reply = fmt.Sprintf("已将 %d 个游戏移出主动匹配黑名单", len(appIDs))
		}
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
		arg := strings.TrimSpace(command[len("fbrm"):])
		if strings.EqualFold(arg, "all") {
			if err := b.FarmingBlacklistClear(); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = "已清空挂机黑名单"
			}
			break
		}
		appID, err := strconv.ParseUint(arg, 10, 32)
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
		arg := strings.TrimSpace(command[len("fqrm"):])
		if strings.EqualFold(arg, "all") {
			if err := b.FarmingQueueClear(); err != nil {
				reply = fmt.Sprintf("保存失败: %v", err)
			} else {
				reply = "已清空挂机队列"
			}
			break
		}
		appID, err := strconv.ParseUint(arg, 10, 32)
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
			reply = "用法: !redeem <激活码> [flags]，flags: ff(转发失败) fawk(假定钱包码) fd/sd/sf/si/skmg/fkmg/v"
		} else {
			fields := strings.Fields(arg)
			keys := []string{fields[0]}
			flagFields := fields[1:]
			flags, err := ParseRedeemFlags(flagFields)
			if err != nil {
				reply = fmt.Sprintf("无效 flag: %v", err)
				break
			}
			resp, err := b.RedeemWithFlags(keys, flags)
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
			progress := ""
			if g.TotalCards > 0 {
				dropped := g.TotalCards - g.CardsRemaining
				if dropped < 0 {
					dropped = 0
				}
				pct := 0
				if g.TotalCards > 0 {
					pct = dropped * 100 / g.TotalCards
				}
				progress = fmt.Sprintf("（已掉落 %d/%d，%d%%）", dropped, g.TotalCards, pct)
			}
			text += fmt.Sprintf(" | %s(%d):剩余 %d 张%s", g.Name, g.AppID, g.CardsRemaining, progress)
		}
	}
	if b.cardsFarmer != nil {
		if next := b.cardsFarmer.NextCheckAt(); !next.IsZero() {
			text += fmt.Sprintf(" | 下次检查: %s", next.Local().Format("15:04:05"))
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

// containsString 判断字符串切片是否包含目标。
func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// parseAppIDs 解析逗号分隔的 appid 列表。
func parseAppIDs(text string) ([]uint32, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(fields) == 0 {
		return nil, fmt.Errorf("无 appid")
	}
	appIDs := make([]uint32, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseUint(f, 10, 32)
		if err != nil || v == 0 {
			return nil, fmt.Errorf("%q", f)
		}
		appIDs = append(appIDs, uint32(v))
	}
	return appIDs, nil
}

// commandHelp 记录每个命令的详细用法（!help <命令> 时展示）。
var commandHelp = map[string]string{
	"status":       "查看本机器人的连接状态与挂机进度",
	"pause":        "暂停卡牌挂机（!pause~ 永久暂停）",
	"resume":       "恢复卡牌挂机",
	"farm":         "立即重新检查并开始挂机",
	"restart":      "重启本机器人",
	"unpack":       "打开库存中所有补充包",
	"2fa":          "显示当前 Steam 2FA 代码",
	"2faok":        "接受全部待确认的 Steam 确认项",
	"2fano":        "拒绝全部待确认的 Steam 确认项",
	"mafile":       "显示移动令牌文件（.maFile）内容",
	"password":     "修改 Steam 密码: !password <新密码>",
	"keys":         "查看后台兑换列表中的密钥",
	"batch":        "立即批量兑换: !batch <key...>；无参数时从后台兑换列表加载并兑换",
	"inventory":    "查看库存: !inventory",
	"play":         "设置游玩游戏: !play <appid...>（留空清除）",
	"input":        "提交 Steam Guard/2FA 验证码: !input <code>",
	"privacy":      "修改隐私: !privacy <profile> [ownedGames] [playtime] [friendsList] [inventory] [inventoryGifts] [comments]，1=私密 2=好友 3=公开",
	"bgr":          "添加密钥到后台兑换列表: !bgr <key>",
	"bgrm":         "从后台兑换列表移除: !bgrm <key>",
	"bgrclear":     "清空后台兑换列表",
	"hash":         "计算 SCrypt 哈希（IPC 密码格式）: !hash <text>",
	"encrypt":      "AES 加密文本: !encrypt <text>",
	"reset":        "重置登录 Key 并重新登录",
	"mab":          "查看主动匹配黑名单",
	"mabadd":       "添加主动匹配黑名单: !mabadd <appid[,appid...]>",
	"mabrm":        "移除主动匹配黑名单: !mabrm <appid[,appid...]|all>",
	"fb":           "查看挂机黑名单",
	"fbadd":        "添加挂机黑名单: !fbadd <appid>",
	"fbrm":         "移除挂机黑名单: !fbrm <appid|all>",
	"fq":           "查看挂机队列",
	"fqadd":        "添加挂机队列: !fqadd <appid>",
	"fqrm":         "移除挂机队列: !fqrm <appid|all>",
	"loot":         "把可转移物品发给指定用户: !loot <SteamID64>",
	"transfer":     "把可转移物品转给另一个机器人: !transfer <BotName>",
	"closetrades":  "关闭所有进行中的交易报价",
	"nickname":     "修改 Steam 昵称: !nickname <名称>",
	"addlicense":   "添加免费许可证: !addlicense <sub/xxx>",
	"checkappbeta": "检查应用测试版密码: !checkappbeta <appid> <password>",
	"owns":         "检查是否拥有应用: !owns <appid>",
	"redeem":       "兑换激活码: !redeem <key> [flags]，flags: ff/fawk/fd/fkmg/sawk/sd/sf/si/skmg/v",
	"level":        "查询 Steam 等级",
	"points":       "查询 Steam 积分余额",
	"stats":        "查看机器人统计信息",
	"version":      "显示 ASF Go 版本",
	"update":       "检查并应用 ASF Go 更新",
	"exit":         "退出 ASF（仅 Owner）",
	"help":         "显示帮助: !help [命令]",
}
