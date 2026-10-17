package ipc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/bot"
	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/store"
	"github.com/skip2/go-qrcode"
)

func (s *Server) handleBot(w http.ResponseWriter, r *http.Request) {
	// 从原始路径提取 bot 名（保留大小写，对齐原版大小写敏感行为）
	origPath, _ := r.Context().Value(originalPathKey{}).(string)
	if origPath == "" {
		origPath = r.URL.Path
	}
	lowerPath := strings.ToLower(origPath)
	idx := strings.Index(lowerPath, "/api/bot/")
	var parts []string
	if idx >= 0 {
		rest := origPath[idx+len("/api/bot/"):]
		parts = strings.Split(rest, "/")
	} else {
		path := strings.TrimPrefix(r.URL.Path, "/api/bot/")
		path = strings.Trim(path, "/")
		parts = strings.Split(path, "/")
	}
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "缺少 Bot 名称"})
		return
	}
	botName := parts[0]

	// GET /api/bot/ASF：返回所有 Bot 的映射（原版 Bot.GetBots 对 "ASF"/"@ALL" 的特殊处理，ASF-ui 依赖此端点轮询列表）
	if len(parts) == 1 && strings.EqualFold(botName, "ASF") && r.Method == http.MethodGet {
		result := make(map[string]any)
		for name, bot := range s.asf.Bots() {
			result[name] = botStatusMap(bot)
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": result})
		return
	}

	// POST /api/bot/{name}：创建或更新 Bot 配置（原版 BotPost 允许新建，ASF-ui 新建机器人依赖此端点）
	if r.Method == http.MethodPost && len(parts) == 1 {
		var req struct {
			BotConfig *config.BotConfig `json:"botConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BotConfig == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效的 Bot 配置"})
			return
		}
		if err := s.asf.UpdateBotConfig(botName, req.BotConfig); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
			return
		}
		// 原版 BotPost 返回 GenericResponse<IReadOnlyDictionary<string, bool>>，Result 为 botName -> 是否保存成功
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]bool{botName: true}})
		return
	}

	b, ok := s.asf.Bots()[botName]
	if !ok {
		// 原版 BotController 对不存在的 bot 返回 200 + 空字典（仅 GET 列表场景）
		if r.Method == http.MethodGet && len(parts) == 1 {
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{}})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "Bot 不存在"})
		return
	}

	// 保留兼容：POST /api/bot/{name} 且无 body 时视为空配置（一般不会到达此处）
	if r.Method == http.MethodPost && len(parts) == 1 {
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已保存"})
		return
	}

	// DELETE /api/bot/{name}：删除 Bot（暂不支持）
	if r.Method == http.MethodDelete && len(parts) == 1 {
		writeJSON(w, http.StatusOK, map[string]any{"Success": false, "Message": "暂不支持删除 Bot"})
		return
	}

	if len(parts) >= 2 {
		action := strings.Join(parts[1:], "/")
		// 别名路径归一化（兼容第三方/扫描器使用的非官方路径）
		switch strings.ToLower(action) {
		case "2fa":
			action = "twofactorauthentication"
		case "bgr":
			action = "gamesToRedeeminbackground"
		}

		// 原版路径格式：GET /api/bot/{name}/Inventory/{appID}/{contextID}
		if strings.HasPrefix(strings.ToLower(action), "inventory/") {
			invParts := strings.Split(action, "/")
			appID := uint32(753)
			contextID := uint64(6)
			if len(invParts) >= 2 {
				if v, err := parseUint32(invParts[1]); err == nil {
					appID = v
				}
			}
			if len(invParts) >= 3 {
				if v, err := parseUint64(invParts[2]); err == nil {
					contextID = v
				}
			}
			inv, err := b.GetInventory(appID, contextID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": inv})
			return
		}

		switch key := strings.ToLower(r.Method + " " + action); key {
		case "post pause":
			b.Pause()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已暂停"})
		case "post resume":
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已恢复"})
		case "post restart":
			b.Pause()
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已重启"})
		case "post farm":
			b.Pause()
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已开始挂机"})
		case "get status":
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": botStatusMap(b)})
		case "get qrcode":
			qrURL := b.QRChallengeURL()
			if qrURL == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "该 Bot 未处于 QR 扫码登录状态"})
				return
			}
			png, err := qrcode.Encode(qrURL, qrcode.Medium, 256)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "get twofactorauthentication/confirmations":
			confirmations, err := b.GetConfirmations()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			list := make([]map[string]any, 0, len(confirmations))
			for _, c := range confirmations {
				list = append(list, map[string]any{
					"ID":        uint64(c.ID),
					"Nonce":     uint64(c.Nonce),
					"Type":      c.Type,
					"CreatorID": uint64(c.CreatorID),
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": list})
		case "post start":
			b.Resume()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已启动"})
		case "post stop":
			b.Pause()
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已停止"})
		case "post delete":
			if r.Header.Get("X-ASF-Confirm") != "true" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "需要确认头 X-ASF-Confirm: true 才能删除 Bot"})
				return
			}
			if err := s.asf.RemoveBot(botName); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已删除"})
		case "post removelicense":
			// 移除许可需要 CM 协议（SteamApps.RemoveLicense），暂未实现
			writeJSON(w, http.StatusOK, map[string]any{"Success": false, "Message": "暂不支持移除许可"})
		case "post rename":
			var req struct {
				NewName string `json:"newName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NewName == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "缺少新名称"})
				return
			}
			if err := s.asf.RenameBot(botName, req.NewName); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已重命名"})
		case "get gamestofarm":
			games := b.FarmingGames()
			list := make([]map[string]any, 0, len(games))
			for _, g := range games {
				list = append(list, map[string]any{"appID": g.AppID, "gameName": g.Name, "cardsRemaining": g.CardsRemaining})
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": list})
		case "get farmingblacklist":
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": b.Config.FarmingBlacklist})
		case "post farmingblacklist/add":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingBlacklistAdd(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已添加"})
		case "post farmingblacklist/remove":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingBlacklistRemove(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已移除"})
		case "get farmingqueue":
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": b.Config.FarmingQueue})
		case "post farmingqueue/add":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingQueueAdd(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已添加"})
		case "post farmingqueue/remove":
			appID, err := parseUint32(r.URL.Query().Get("appid"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
				return
			}
			if err := b.FarmingQueueRemove(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已移除"})
		case "post unpack":
			opened, err := b.UnpackBoosters()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"opened": opened}})
		case "get gamestoredeeminbackground":
			// BGR：读取后台兑换文件（原版 GamesToRedeemInBackground）
			bgrPath := filepath.Join(s.asf.ConfigPath, botName+".games-to-redeem-in-background")
			bgrData, bgrErr := os.ReadFile(bgrPath)
			var bgrLines []string
			if bgrErr == nil {
				bgrLines = strings.Fields(string(bgrData))
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": bgrLines})
		case "post gamestoredeeminbackground":
			var req struct {
				GamesToRedeemInBackground []string `json:"gamesToRedeemInBackground"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			bgrPath := filepath.Join(s.asf.ConfigPath, botName+".games-to-redeem-in-background")
			if err := os.WriteFile(bgrPath, []byte(strings.Join(req.GamesToRedeemInBackground, "\n")), 0o600); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已保存"})
		case "delete gamestoredeeminbackground":
			bgrPath := filepath.Join(s.asf.ConfigPath, botName+".games-to-redeem-in-background")
			_ = os.Remove(bgrPath)
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "已删除"})
		case "post input":
			// 提交登录验证码（对应原版 BotInputRequest，原版支持 SteamGuard/2FA 输入）
			var req struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Value) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "缺少验证码 value"})
				return
			}
			if err := b.SubmitInput(req.Value); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "验证码已设置，正在重新登录"})
		case "get 2fa":
			code, err := b.TwoFactorCode()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"code": code}})
		case "get twofactorauthentication/token":
			code, err := b.TwoFactorCode()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"token": code}})
		case "post twofactorauthentication":
			var req struct {
				SharedSecret   string `json:"SharedSecret"`
				IdentitySecret string `json:"IdentitySecret"`
				DeviceID       string `json:"DeviceID"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			if err := b.ImportAuthenticator(req.SharedSecret, req.IdentitySecret, req.DeviceID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "移动令牌已导入"})
		case "get inventory":
			appID := uint32(753)
			contextID := uint64(6)
			if v := r.URL.Query().Get("appid"); v != "" {
				if parsed, err := parseUint32(v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "appid 无效"})
					return
				} else {
					appID = parsed
				}
			}
			if v := r.URL.Query().Get("contextid"); v != "" {
				if parsed, err := parseUint64(v); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "contextid 无效"})
					return
				} else {
					contextID = parsed
				}
			}
			inv, err := b.GetInventory(appID, contextID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": inv})
		case "post twofactorauthentication/confirmations":
			if !b.HasAuthenticator() {
				writeJSON(w, http.StatusOK, map[string]any{"Success": false, "Message": "未配置移动令牌"})
				return
			}
			var req struct {
				Accept bool `json:"Accept"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "无效请求"})
				return
			}
			if !b.IsConnected() {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "Bot 未登录"})
				return
			}
			ws := b.WebSessionOrNil()
			if ws == nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "Web 会话不可用"})
				return
			}
			code, err := b.Auth().GenerateConfirmationCode("conf")
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			deviceID := b.Auth().DeviceID
			if deviceID == "" {
				deviceID = "android:asf-go"
			}
			now := time.Now().Unix()
			confirmations, err := store.GetConfirmations(ws, b.SteamID(), deviceID, code, now)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			if len(confirmations) == 0 {
				writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "没有待确认项", "Result": []any{}})
				return
			}
			ok, err := store.HandleConfirmations(ws, b.SteamID(), deviceID, code, now, confirmations, req.Accept)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			list := make([]map[string]any, 0, len(confirmations))
			for _, c := range confirmations {
				list = append(list, map[string]any{
					"ID":        uint64(c.ID),
					"Nonce":     uint64(c.Nonce),
					"Type":      c.Type,
					"CreatorID": uint64(c.CreatorID),
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": ok, "Message": "已处理确认", "Result": list})
		case "post mafile":
			var req struct {
				SharedSecret   string `json:"shared_secret"`
				IdentitySecret string `json:"identity_secret"`
				DeviceID       string `json:"device_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			if err := b.ImportAuthenticator(req.SharedSecret, req.IdentitySecret, req.DeviceID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "移动令牌已导入"})
		case "get level":
			level, err := b.SteamLevel()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"level": level}})
		case "post loot":
			var req struct {
				TargetSteamID uint64 `json:"target_steamid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			offerID, err := b.Loot(req.TargetSteamID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{"offer_id": offerID}})
		case "post addlicense":
			var req struct {
				Identifiers []string `json:"identifiers"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			lines := make([]string, 0, len(req.Identifiers))
			for _, ident := range req.Identifiers {
				resp, err := b.AddLicense(ident)
				if err != nil {
					lines = append(lines, fmt.Sprintf("%s 失败: %v", ident, err))
				} else {
					lines = append(lines, resp)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": lines})
		case "post redeem":
			var req struct {
				Keys []string `json:"keys"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			resp, err := b.Redeem(req.Keys)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": resp})
		default:
			writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "未知操作"})
		}
		return
	}

	// 原版 BotGet 返回 GenericResponse<IReadOnlyDictionary<string, Bot>>，Result 是 {botName: bot} 字典
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": map[string]any{botName: botStatusMap(b)}})
}

// handleBots 返回机器人列表/统计（兼容 ASF-ui 与第三方工具的 /api/bots 系列路径）。

func (s *Server) handleBots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 GET"})
		return
	}
	bots := s.asf.Bots()
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/bots"), "/")

	switch sub {
	case "", "bots", "detectbots":
		result := make([]map[string]any, 0, len(bots))
		for name, b := range bots {
			result = append(result, map[string]any{
				"name":   name,
				"status": botStatusMap(b),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": result})
	case "count":
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": len(bots)})
	case "botsfarmingcount":
		count := 0
		for _, b := range bots {
			if b.IsFarming() {
				count++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": count})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"Success": false, "Message": "未知操作"})
	}
}

// resolveCommandBot 解析原版命令的"目标 bot"：第二个 token 若匹配已知 bot 则使用之，
// 否则回退到全局配置的 DefaultBot（再退化为第一个启用的 bot）。

func (s *Server) resolveCommandBot(fields []string) (*bot.Bot, int) {
	if len(fields) >= 2 {
		if b, ok := s.asf.Bots()[fields[1]]; ok {
			return b, 2
		}
	}
	if s.asf.Config.DefaultBot != "" {
		if b, ok := s.asf.Bots()[s.asf.Config.DefaultBot]; ok {
			return b, 1
		}
	}
	for _, b := range s.asf.Bots() {
		if b.Config.Enabled {
			return b, 1
		}
	}
	for _, b := range s.asf.Bots() {
		return b, 1
	}
	return nil, 1
}
