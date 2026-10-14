// Package localization 移植自 ArchiSteamFarm.Localization，提供消息文本。
package localization

import (
	"fmt"
	"strings"
)

// Language 语言标识。
type Language string

const (
	English Language = "en-US"
	Chinese Language = "zh-CN"
)

// Strings 包含当前语言的文本映射。
type Strings struct {
	lang Language
}

// New 创建本地化对象。
func New(lang Language) *Strings {
	if lang == "" {
		lang = English
	}
	return &Strings{lang: lang}
}

// Get 返回指定键的文本。
func (s *Strings) Get(key string) string {
	if table, ok := tables[s.lang]; ok {
		if v, ok := table[key]; ok {
			return v
		}
	}
	// 回退英文
	if v, ok := tables[English][key]; ok {
		return v
	}
	return key
}

// Format 返回格式化后的文本（支持 {0} {1} 占位符）。
func (s *Strings) Format(key string, args ...any) string {
	text := s.Get(key)
	for i, arg := range args {
		placeholder := fmt.Sprintf("{%d}", i)
		text = strings.ReplaceAll(text, placeholder, fmt.Sprintf("%v", arg))
	}
	return text
}

// 默认英文与中文文本表。
var tables = map[Language]map[string]string{
	English: {
		"BotStarted":        "Bot {0} has started",
		"BotStopped":        "Bot {0} has stopped",
		"BotPaused":         "Bot {0} farming paused",
		"BotResumed":        "Bot {0} farming resumed",
		"LoggedOn":          "Bot {0} logged on",
		"IdlingStarted":     "CardsFarmer started",
		"IdlingStopped":     "CardsFarmer stopped",
		"NoGamesToFarm":     "No games with card drops remaining",
		"FarmingGame":       "Farming {0} (AppID {1}), {2} cards remaining",
		"CommandPrefix":     "Available commands: status, pause, resume, help",
		"UnknownCommand":    "Unknown command. Send !help for help.",
		"UpdateAvailable":   "Upstream ArchiSteamFarm latest version: {0}",
		"WebSessionReady":   "Web session ready",
		"ConnectionTimeout": "Steam login timed out",
	},
	Chinese: {
		"BotStarted":        "Bot {0} 已启动",
		"BotStopped":        "Bot {0} 已停止",
		"BotPaused":         "Bot {0} 已暂停挂机",
		"BotResumed":        "Bot {0} 已恢复挂机",
		"LoggedOn":          "Bot {0} 已登录",
		"IdlingStarted":     "CardsFarmer 已启动",
		"IdlingStopped":     "CardsFarmer 已停止",
		"NoGamesToFarm":     "没有发现仍有卡牌掉落的游戏",
		"FarmingGame":       "正在挂机 {0} (AppID {1})，剩余卡牌 {2}",
		"CommandPrefix":     "可用命令: status, pause, resume, help",
		"UnknownCommand":    "未知命令。发送 !help 查看帮助。",
		"UpdateAvailable":   "上游 ArchiSteamFarm 最新版本: {0}",
		"WebSessionReady":   "Web 会话已就绪",
		"ConnectionTimeout": "Steam 登录超时",
	},
}
