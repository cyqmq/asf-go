package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// mabFilePath 返回主动匹配黑名单持久化文件路径。
func (b *Bot) mabFilePath() string {
	return filepath.Join(b.configDir, b.Name+".mab.json")
}

// loadMAB 从磁盘加载主动匹配黑名单。
func (b *Bot) loadMAB() {
	data, err := os.ReadFile(b.mabFilePath())
	if err != nil {
		return
	}
	var appIDs []uint32
	if err := json.Unmarshal(data, &appIDs); err != nil {
		b.Log.Warn("解析主动匹配黑名单失败: %v", err)
		return
	}
	b.mabMutex.Lock()
	b.mabAppIDs = appIDs
	b.mabMutex.Unlock()
}

// saveMAB 持久化主动匹配黑名单。
func (b *Bot) saveMAB() error {
	b.mabMutex.Lock()
	defer b.mabMutex.Unlock()
	data, err := json.Marshal(b.mabAppIDs)
	if err != nil {
		return fmt.Errorf("序列化主动匹配黑名单失败: %w", err)
	}
	return os.WriteFile(b.mabFilePath(), data, 0o600)
}

// MatchActivelyBlacklist 返回主动匹配黑名单副本。
func (b *Bot) MatchActivelyBlacklist() []uint32 {
	b.mabMutex.RLock()
	defer b.mabMutex.RUnlock()
	out := make([]uint32, len(b.mabAppIDs))
	copy(out, b.mabAppIDs)
	return out
}

// MatchActivelyBlacklistAdd 添加游戏到主动匹配黑名单并保存。
func (b *Bot) MatchActivelyBlacklistAdd(appIDs []uint32) error {
	b.mabMutex.Lock()
	changed := false
	for _, appID := range appIDs {
		found := false
		for _, id := range b.mabAppIDs {
			if id == appID {
				found = true
				break
			}
		}
		if !found {
			b.mabAppIDs = append(b.mabAppIDs, appID)
			changed = true
		}
	}
	b.mabMutex.Unlock()
	if !changed {
		return nil
	}
	return b.saveMAB()
}

// MatchActivelyBlacklistRemove 从主动匹配黑名单移除游戏并保存。appIDs 为空时清空全部。
func (b *Bot) MatchActivelyBlacklistRemove(appIDs []uint32) error {
	b.mabMutex.Lock()
	if len(appIDs) == 0 {
		b.mabAppIDs = nil
		b.mabMutex.Unlock()
		return b.saveMAB()
	}
	removeSet := make(map[uint32]struct{}, len(appIDs))
	for _, appID := range appIDs {
		removeSet[appID] = struct{}{}
	}
	filtered := make([]uint32, 0, len(b.mabAppIDs))
	for _, id := range b.mabAppIDs {
		if _, ok := removeSet[id]; !ok {
			filtered = append(filtered, id)
		}
	}
	b.mabAppIDs = filtered
	b.mabMutex.Unlock()
	return b.saveMAB()
}

// formatMAB 生成 MAB 列表文本。
func (b *Bot) formatMAB() string {
	ids := b.MatchActivelyBlacklist()
	if len(ids) == 0 {
		return "主动匹配黑名单为空"
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	strs := make([]string, 0, len(ids))
	for _, id := range ids {
		strs = append(strs, fmt.Sprintf("%d", id))
	}
	return "主动匹配黑名单: " + joinStrings(strs, ", ")
}

// joinStrings 用分隔符连接字符串切片。
func joinStrings(list []string, sep string) string {
	out := ""
	for i, s := range list {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}
