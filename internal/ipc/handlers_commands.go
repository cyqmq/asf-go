package ipc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"Success": false, "Message": "仅支持 POST"})
		return
	}
	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": err.Error()})
		return
	}
	cmd := strings.TrimSpace(req.Command)
	prefix := s.asf.Config.CommandPrefix
	if prefix == "" {
		prefix = "!"
	}
	if strings.HasPrefix(cmd, prefix) {
		cmd = strings.TrimSpace(strings.TrimPrefix(cmd, prefix))
	}
	if cmd == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Success": false, "Message": "命令不能为空"})
		return
	}

	fields := strings.Fields(cmd)
	lower := strings.ToLower(fields[0])
	respond := func(success bool, message string) {
		writeJSON(w, http.StatusOK, map[string]any{"Success": success, "Message": message, "Result": message})
	}

	switch lower {
	case "help":
		respond(true, "可用命令: status, pause, resume, farm, restart, reset, redeem, addlicense, owns, play, nickname, level, points, games, 2fa, confirmations, update, exit, help")
	case "status", "sa":
		if len(fields) >= 2 && strings.EqualFold(fields[1], "ASF") {
			st := s.asfStatus()
			respond(true, fmt.Sprintf("ASF %s | Bot 数: %d", st["version"], st["bots"]))
			return
		}
		if len(fields) >= 2 {
			if b, ok := s.asf.Bots()[fields[1]]; ok {
				writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": botStatusMap(b)})
				return
			}
		}
		result := make(map[string]any)
		lines := make([]string, 0, len(s.asf.Bots()))
		for name, b := range s.asf.Bots() {
			result[name] = botStatusMap(b)
			lines = append(lines, b.String())
		}
		writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Result": result, "Message": strings.Join(lines, "\n")})
	case "pause":
		b, idx := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		_ = idx
		b.Pause()
		respond(true, fmt.Sprintf("Bot %s 已暂停", b.Name))
	case "resume":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		b.Resume()
		respond(true, fmt.Sprintf("Bot %s 已恢复", b.Name))
	case "farm":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		b.Resume()
		respond(true, fmt.Sprintf("Bot %s 已开始挂机", b.Name))
	case "restart", "reset":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		b.Pause()
		b.Resume()
		respond(true, fmt.Sprintf("Bot %s 已重启", b.Name))
	case "redeem":
		b, start := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		if start >= len(fields) {
			respond(false, "用法: redeem <Bot> <Key> [Key2] ...")
			return
		}
		keys := fields[start:]
		msg, err := b.Redeem(keys)
		if err != nil {
			respond(false, err.Error())
			return
		}
		respond(true, msg)
	case "addlicense":
		b, start := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		if start >= len(fields) {
			respond(false, "用法: addlicense <Bot> <SubID>")
			return
		}
		msg, err := b.AddLicense(fields[start])
		if err != nil {
			respond(false, err.Error())
			return
		}
		respond(true, msg)
	case "owns":
		b, start := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		if start >= len(fields) {
			respond(false, "用法: owns <Bot> <AppID>")
			return
		}
		msg, err := b.Owns(fields[start:])
		if err != nil {
			respond(false, err.Error())
			return
		}
		respond(true, msg)
	case "play":
		b, start := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		if start >= len(fields) {
			respond(false, "用法: play <Bot> <AppID>")
			return
		}
		respond(false, "暂不支持 play 命令")
	case "nickname":
		b, start := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		if start >= len(fields) {
			respond(false, "用法: nickname <Bot> <昵称>")
			return
		}
		nickname := strings.Join(fields[start:], " ")
		if err := b.SetNickname(nickname); err != nil {
			respond(false, err.Error())
			return
		}
		respond(true, fmt.Sprintf("Bot %s 昵称已修改", b.Name))
	case "level":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		level, err := b.SteamLevel()
		if err != nil {
			respond(false, fmt.Sprintf("查询等级失败: %v", err))
			return
		}
		respond(true, fmt.Sprintf("Bot %s 的 Steam 等级: %d", b.Name, level))
	case "points", "balance":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		points, err := b.PointsBalance()
		if err != nil {
			respond(false, fmt.Sprintf("查询积分失败: %v", err))
			return
		}
		respond(true, fmt.Sprintf("Bot %s 的 Steam 积分: %d", b.Name, points))
	case "games":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		games := b.FarmingGames()
		if len(games) == 0 {
			respond(true, fmt.Sprintf("Bot %s 当前无挂机游戏", b.Name))
			return
		}
		lines := make([]string, 0, len(games))
		for _, g := range games {
			lines = append(lines, fmt.Sprintf("%s (%d): %d 张卡", g.Name, g.AppID, g.CardsRemaining))
		}
		respond(true, strings.Join(lines, "\n"))
	case "2fa":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		code, err := b.TwoFactorCode()
		if err != nil {
			respond(false, err.Error())
			return
		}
		respond(true, fmt.Sprintf("Bot %s 的 2FA 代码: %s", b.Name, code))
	case "confirmations":
		b, _ := s.resolveCommandBot(fields)
		if b == nil {
			respond(false, "没有可用 Bot")
			return
		}
		confirmations, err := b.GetConfirmations()
		if err != nil {
			respond(false, err.Error())
			return
		}
		if len(confirmations) == 0 {
			respond(true, fmt.Sprintf("Bot %s 无待确认项", b.Name))
			return
		}
		respond(true, fmt.Sprintf("Bot %s 有 %d 个待确认项", b.Name, len(confirmations)))
	case "update":
		message, err := s.asf.Update()
		if err != nil {
			respond(false, err.Error())
			return
		}
		respond(true, message)
	case "exit":
		respond(false, "已禁止远程退出，请在服务器上管理 ASF 进程")
	default:
		respond(false, "未知命令: "+fields[0])
	}
}
