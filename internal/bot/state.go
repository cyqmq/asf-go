package bot

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// stateFile 保存 Bot 的运行时状态快照，用于崩溃/重启后快速恢复（对应原版 ASF.db 的轻量替代）。
// 挂机游戏列表可以从 Steam 徽章页面实时重建，此处保存仅用于加速恢复，不依赖也不会阻塞启动。
type stateFile struct {
	FarmingGames []GameInfo `json:"FarmingGames"`
}

// statePath 返回状态文件路径。
func (b *Bot) statePath() string {
	return filepath.Join(b.configDir, b.Name+".state.json")
}

// saveState 保存当前挂机游戏列表快照。
func (b *Bot) saveState(games []GameInfo) {
	state := stateFile{FarmingGames: games}
	data, err := json.MarshalIndent(&state, "", "\t")
	if err != nil {
		return
	}
	_ = os.WriteFile(b.statePath(), data, 0o600)
}

// loadState 读取上次保存的挂机游戏列表（不存在或损坏时返回 nil）。
func (b *Bot) loadState() []GameInfo {
	data, err := os.ReadFile(b.statePath())
	if err != nil {
		return nil
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	return state.FarmingGames
}
