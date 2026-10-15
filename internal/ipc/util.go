package ipc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/asf-go/asf/internal/bot"
	"github.com/asf-go/asf/internal/core"
)

func botStatusMap(b *bot.Bot) map[string]any {
	games := make([]map[string]any, 0)
	for _, g := range b.FarmingGames() {
		games = append(games, map[string]any{
			"AppID":          g.AppID,
			"GameName":       g.Name,
			"CardsRemaining": g.CardsRemaining,
		})
	}
	cardsFarmer := map[string]any{
		"Paused":              !b.IsFarming(),
		"GamesToFarm":         games,
		"TimeRemaining":       "00:00:00",
		"CurrentGamesFarming": games,
	}
	qrURL := b.QRChallengeURL()
	requiredInput := 0
	if qrURL != "" {
		requiredInput = 8 // ASF.EUserInputType.QrCodeLogin
	}
	return map[string]any{
		// 原版字段（ASF-ui 依赖这些键名判断在线/挂卡状态）
		"BotName":                        b.Name,
		"CardsFarmer":                    cardsFarmer,
		"GamesToRedeemInBackgroundCount": 0,
		"HasMobileAuthenticator":         b.HasAuthenticator(),
		"IsConnectedAndLoggedOn":         b.IsConnected(),
		"IsPlayingPossible":              b.IsConnected(),
		"PublicIP":                       nil,
		"s_SteamID":                      strconv.FormatUint(b.SteamID(), 10),
		"AccountFlags":                   0,
		"AvatarHash":                     nil,
		"BotConfig":                      b.Config,
		"KeepRunning":                    b.Config.Enabled,
		"Nickname":                       nil,
		"QrChallengeURL":                 qrURL,
		"RequiredInput":                  requiredInput,
		"SteamID":                        b.SteamID(),
		"WalletBalance":                  0,
		"WalletBalanceDelayed":           0,
		"WalletCurrency":                 nil,
		// 兼容旧字段（历史消费者可能仍在使用）
		"name":             b.Name,
		"connected":        b.IsConnected(),
		"enabled":          b.Config.Enabled,
		"onlineStatus":     b.Config.OnlineStatus,
		"steamLogin":       b.Config.SteamLogin,
		"gamesToFarm":      games,
		"cardsFarmer":      cardsFarmer,
		"farmingBlacklist": b.Config.FarmingBlacklist,
		"farmingQueue":     b.Config.FarmingQueue,
		"access":           "None",
	}
}

func (s *Server) asfStatus() map[string]any {
	return map[string]any{
		"version": core.Version,
		"bots":    len(s.asf.Bots()),
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func parseUint32(s string) (uint32, error) {
	var v uint32
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, err
	}
	return v, nil
}

func parseUint64(s string) (uint64, error) {
	var v uint64
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, err
	}
	return v, nil
}
