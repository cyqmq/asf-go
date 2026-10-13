// Package bot 的游戏标识符解析，复刻 ArchiSteamFarm Steam/Integration/SteamUtilities.TryParseGameIdentifier。
package bot

import (
	"net/url"
	"strconv"
	"strings"
)

// GameIdentifierType 对应原版 EGameIdentifier 枚举。
type GameIdentifierType byte

const (
	GameIdentifierApplication GameIdentifierType = iota
	GameIdentifierPackage
	GameIdentifierName
	GameIdentifierRegex
)

// ParseGameIdentifier 解析游戏标识符输入，返回类型与 ID。
// 支持格式：
//
//	https://store.steampowered.com/app/<appid>    → Application
//	https://store.steampowered.com/sub/<subid>    → Package
//	steam://launch|install|run/<appid>           → Application
//	app/<appid> 或 a/<appid>                      → Application
//	sub/<subid> 或 s/<subid>                      → Package
//	裸数字                                        → 按 defaultType
func ParseGameIdentifier(input string, defaultType GameIdentifierType) (GameIdentifierType, uint32, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, 0, false
	}

	// URL 形式
	if u, err := url.Parse(input); err == nil && u.Scheme != "" {
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			if strings.EqualFold(u.Host, "store.steampowered.com") {
				segments := strings.Split(strings.Trim(u.Path, "/"), "/")
				if len(segments) < 2 {
					break
				}
				var idType GameIdentifierType
				recognized := false
				switch strings.ToUpper(segments[0]) {
				case "APP":
					idType = GameIdentifierApplication
					recognized = true
				case "SUB":
					idType = GameIdentifierPackage
					recognized = true
				}
				if !recognized {
					break
				}
				if appID, err := strconv.ParseUint(segments[1], 10, 32); err == nil && appID > 0 {
					return idType, uint32(appID), true
				}
			}
		case "steam":
			if strings.EqualFold(u.Host, "launch") || strings.EqualFold(u.Host, "install") || strings.EqualFold(u.Host, "run") {
				segments := strings.Split(strings.Trim(u.Path, "/"), "/")
				if len(segments) >= 1 {
					if appID, err := strconv.ParseUint(segments[0], 10, 32); err == nil && appID > 0 {
						return GameIdentifierApplication, uint32(appID), true
					}
				}
			}
		}
	}

	// ASF 格式：<格式>/<值>
	if slashIndex := strings.Index(input, "/"); slashIndex > 0 && slashIndex < len(input)-1 {
		inputType := strings.ToUpper(input[:slashIndex])
		value := input[slashIndex+1:]
		switch inputType {
		case "A", "APP":
			if id, err := strconv.ParseUint(value, 10, 32); err == nil && id > 0 {
				return GameIdentifierApplication, uint32(id), true
			}
			return 0, 0, false
		case "S", "SUB":
			if id, err := strconv.ParseUint(value, 10, 32); err == nil && id > 0 {
				return GameIdentifierPackage, uint32(id), true
			}
			return 0, 0, false
		case "N", "NAME":
			return GameIdentifierName, 0, true
		case "R", "REGEX":
			return GameIdentifierRegex, 0, true
		}
	}

	// 裸数字按默认类型
	switch defaultType {
	case GameIdentifierApplication, GameIdentifierPackage:
		if id, err := strconv.ParseUint(input, 10, 32); err == nil && id > 0 {
			return defaultType, uint32(id), true
		}
	}

	return 0, 0, false
}