// Package ipc 的 Type/Structure 端点实现。
// 原版 ASF 的 TypeController 返回 .NET 类型的反射元数据，ASF-ui 配置编辑器依赖
// GET /api/type/{FQN} 获取字段类型映射、GET /api/structure/{FQN} 获取默认值。
// 这里用 Go 配置结构体反射 + 原版 C# 类型名映射表生成等价响应，避免修改 ASF-ui。
package ipc

import (
	"reflect"
	"strings"

	"github.com/asf-go/asf/internal/config"
)

// typeProperties 对应原版 TypeProperties（BaseType/CustomAttributes/UnderlyingType）。
type typeProperties struct {
	BaseType         string   `json:"BaseType"`
	CustomAttributes []string `json:"CustomAttributes,omitempty"`
	UnderlyingType   string   `json:"UnderlyingType,omitempty"`
}

// typeResponse 对应原版 TypeResponse（Body + Properties）。
type typeResponse struct {
	Body       map[string]string `json:"Body"`
	Properties typeProperties    `json:"Properties"`
}

// enumInfo 描述一个 C# 枚举类型的元数据。
type enumInfo struct {
	Underlying string
	Flags      bool
	Values     map[string]string
}

// customAttributes 返回枚举的 CustomAttributes（带 [Flags] 时输出 System.FlagsAttribute）。
func (e enumInfo) customAttributes() []string {
	if e.Flags {
		return []string{"System.FlagsAttribute"}
	}
	return nil
}

// csFieldTypeOverrides 记录 Go 配置结构体中无法由反射直接推断的 C# 类型映射
// （枚举字段、位标志、原版使用不同容器类型的字段）。键为 JSON 字段名。
// 容器类型名遵循原版 GetUnifiedName 格式：`类型名 + 反引号arity + [参数1][参数2]...`。
var csFieldTypeOverrides = map[string]string{
	// BotConfig
	"BotBehaviour":         "ArchiSteamFarm.Steam.Storage.BotConfig+EBotBehaviour",
	"CompleteTypesToSend":  "System.Collections.Immutable.ImmutableHashSet`1[ArchiSteamFarm.Steam.Data.EAssetType]",
	"FarmingOrders":        "System.Collections.Immutable.ImmutableList`1[ArchiSteamFarm.Steam.Storage.BotConfig+EFarmingOrder]",
	"FarmingPreferences":   "ArchiSteamFarm.Steam.Storage.BotConfig+EFarmingPreferences",
	"GamingDeviceType":     "SteamKit2.EGamingDeviceType",
	"LootableTypes":        "System.Collections.Immutable.ImmutableHashSet`1[ArchiSteamFarm.Steam.Data.EAssetType]",
	"MatchableTypes":       "System.Collections.Immutable.ImmutableHashSet`1[ArchiSteamFarm.Steam.Data.EAssetType]",
	"OnlineFlags":          "SteamKit2.EPersonaStateFlag",
	"OnlineStatus":         "SteamKit2.EPersonaState",
	"PasswordFormat":       "ArchiSteamFarm.Core.ArchiCryptoHelper+ECryptoMethod",
	"RedeemingPreferences": "ArchiSteamFarm.Steam.Storage.BotConfig+ERedeemingPreferences",
	"RemoteCommunication":  "ArchiSteamFarm.Steam.Storage.BotConfig+ERemoteCommunication",
	"SteamUserPermissions": "System.Collections.Immutable.ImmutableDictionary`2[System.UInt64][ArchiSteamFarm.Steam.Storage.BotConfig+EAccess]",
	"TradingPreferences":   "ArchiSteamFarm.Steam.Storage.BotConfig+ETradingPreferences",
	"TransferableTypes":    "System.Collections.Immutable.ImmutableHashSet`1[ArchiSteamFarm.Steam.Data.EAssetType]",
	"UserInterfaceMode":    "SteamKit2.EUIMode",
	// GlobalConfig
	"Blacklist":         "System.Collections.Immutable.ImmutableHashSet`1[System.UInt32]",
	"IPCPasswordFormat": "ArchiSteamFarm.Core.ArchiCryptoHelper+EHashingMethod",
	"OptimizationMode":  "ArchiSteamFarm.Storage.GlobalConfig+EOptimizationMode",
	"PluginsUpdateList": "System.Collections.Immutable.ImmutableHashSet`1[System.String]",
	"PluginsUpdateMode": "ArchiSteamFarm.Storage.GlobalConfig+EPluginsUpdateMode",
	"SteamProtocols":    "SteamKit2.ProtocolTypes",
	"UpdateChannel":     "ArchiSteamFarm.Storage.GlobalConfig+EUpdateChannel",
}

// csEnumTypes 收录 ASF-ui 配置编辑器会请求的 C# 枚举类型。
var csEnumTypes = map[string]enumInfo{
	"ArchiSteamFarm.Core.ArchiCryptoHelper+ECryptoMethod": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"PlainText": "0", "AES": "1", "ProtectedDataForCurrentUser": "2", "EnvironmentVariable": "3", "File": "4",
		},
	},
	"ArchiSteamFarm.Core.ArchiCryptoHelper+EHashingMethod": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"PlainText": "0", "SCrypt": "1", "Pbkdf2": "2",
		},
	},
	"ArchiSteamFarm.Steam.Data.EAssetType": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"Unknown": "0", "BoosterPack": "1", "Emoticon": "2", "FoilTradingCard": "3",
			"ProfileBackground": "4", "TradingCard": "5", "SteamGems": "6", "SaleItem": "7",
			"Consumable": "8", "ProfileModifier": "9", "Sticker": "10", "ChatEffect": "11",
			"MiniProfileBackground": "12", "AvatarProfileFrame": "13", "AnimatedAvatar": "14",
			"KeyboardSkin": "15", "StartupVideo": "16",
		},
	},
	"ArchiSteamFarm.Steam.Storage.BotConfig+EAccess": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"None": "0", "FamilySharing": "1", "Operator": "2", "Master": "3",
		},
	},
	"ArchiSteamFarm.Steam.Storage.BotConfig+EBotBehaviour": {
		Underlying: "System.Byte",
		Flags:      true,
		Values: map[string]string{
			"None": "0", "RejectInvalidFriendInvites": "1", "RejectInvalidTrades": "2",
			"RejectInvalidGroupInvites": "4", "DismissInventoryNotifications": "8",
			"MarkReceivedMessagesAsRead": "16", "MarkBotMessagesAsRead": "32",
			"DisableIncomingTradesParsing": "64", "All": "127",
		},
	},
	"ArchiSteamFarm.Steam.Storage.BotConfig+EFarmingOrder": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"Unordered": "0", "AppIDsAscending": "1", "AppIDsDescending": "2",
			"CardDropsAscending": "3", "CardDropsDescending": "4", "HoursAscending": "5",
			"HoursDescending": "6", "NamesAscending": "7", "NamesDescending": "8",
			"Random": "9", "BadgeLevelsAscending": "10", "BadgeLevelsDescending": "11",
			"RedeemDateTimesAscending": "12", "RedeemDateTimesDescending": "13",
			"MarketableAscending": "14", "MarketableDescending": "15",
		},
	},
	"ArchiSteamFarm.Steam.Storage.BotConfig+EFarmingPreferences": {
		Underlying: "System.UInt16",
		Flags:      true,
		Values: map[string]string{
			"None": "0", "FarmingPausedByDefault": "1", "ShutdownOnFarmingFinished": "2",
			"SendOnFarmingFinished": "4", "FarmPriorityQueueOnly": "8", "SkipRefundableGames": "16",
			"SkipUnplayedGames": "32", "EnableRiskyCardsDiscovery": "64", "AutoUnpackBoosterPacks": "256",
			"All": "383",
		},
	},
	"ArchiSteamFarm.Steam.Storage.BotConfig+ERedeemingPreferences": {
		Underlying: "System.Byte",
		Flags:      true,
		Values: map[string]string{
			"None": "0", "Forwarding": "1", "Distributing": "2", "KeepMissingGames": "4",
			"AssumeWalletKeyOnBadActivationCode": "8", "All": "15",
		},
	},
	"ArchiSteamFarm.Steam.Storage.BotConfig+ERemoteCommunication": {
		Underlying: "System.Byte",
		Flags:      true,
		Values: map[string]string{
			"None": "0", "SteamGroup": "1", "PublicListing": "2", "All": "3",
		},
	},
	"ArchiSteamFarm.Steam.Storage.BotConfig+ETradingPreferences": {
		Underlying: "System.Byte",
		Flags:      true,
		Values: map[string]string{
			"None": "0", "AcceptDonations": "1", "SteamTradeMatcher": "2", "MatchEverything": "4",
			"DontAcceptBotTrades": "8", "MatchActively": "16", "All": "31",
		},
	},
	"ArchiSteamFarm.Storage.GlobalConfig+EOptimizationMode": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"MaxPerformance": "0", "MinMemoryUsage": "1",
		},
	},
	"ArchiSteamFarm.Storage.GlobalConfig+EPluginsUpdateMode": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"Whitelist": "0", "Blacklist": "1",
		},
	},
	"ArchiSteamFarm.Storage.GlobalConfig+EUpdateChannel": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"None": "0", "Stable": "1", "PreRelease": "2",
		},
	},
	"SteamKit2.EPersonaState": {
		Underlying: "System.Byte",
		Values: map[string]string{
			"Offline": "0", "Online": "1", "Busy": "2", "Away": "3", "Snooze": "4",
			"LookingToTrade": "5", "LookingToPlay": "6", "Invisible": "7",
		},
	},
	"SteamKit2.EPersonaStateFlag": {
		Underlying: "System.Int32",
		Flags:      true,
		Values: map[string]string{
			"HasRichPresence": "1", "InJoinableGame": "2", "Golden": "4", "RemotePlayTogether": "8",
			"ClientTypeWeb": "256", "ClientTypeMobile": "512", "ClientTypeTenfoot": "1024",
			"ClientTypeVR": "2048", "LaunchTypeGamepad": "4096", "LaunchTypeCompatTool": "8192",
		},
	},
	"SteamKit2.EGamingDeviceType": {
		Underlying: "System.Int32",
		Values: map[string]string{
			"Unknown": "0", "StandardPC": "1", "ConsoleLegacy": "256", "PS3": "273",
			"Steambox": "288", "Tesla": "320", "Handheld_Legacy": "512", "Phone": "528",
			"Console": "540", "SteamOSGeneric": "541", "SteamDeck": "544", "LegionGoS": "545",
			"Fremont": "546", "VR": "768", "SteamFrame": "769",
		},
	},
	"SteamKit2.EUIMode": {
		Underlying: "System.Int32",
		Values: map[string]string{
			"Unknown": "-1", "VGUI": "0", "Tenfoot": "1", "Mobile": "2", "Web": "3",
			"GamePadUI": "4", "MobileChat": "5", "EmbeddedClient": "6", "DesktopUI": "7",
		},
	},
	"SteamKit2.ProtocolTypes": {
		Underlying: "System.Int32",
		Flags:      true,
		Values: map[string]string{
			"Tcp": "1", "Udp": "2", "WebSocket": "4", "All": "7",
		},
	},
}

// csFieldType 返回 Go 配置结构体字段对应的 C# 统一类型名。
func csFieldType(sf reflect.StructField) string {
	jsonName := sf.Name
	if tag := sf.Tag.Get("json"); tag != "" {
		jsonName = strings.Split(tag, ",")[0]
	}
	if override, ok := csFieldTypeOverrides[jsonName]; ok {
		return override
	}
	return csReflectType(sf.Type)
}

// csReflectType 将 Go 类型映射为 C# 统一类型名（用于未覆盖的普通字段）。
func csReflectType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "System.Boolean"
	case reflect.String:
		return "System.String"
	case reflect.Uint8:
		return "System.Byte"
	case reflect.Uint16:
		return "System.UInt16"
	case reflect.Uint32:
		return "System.UInt32"
	case reflect.Uint64:
		return "System.UInt64"
	case reflect.Uint:
		return "System.UInt32"
	case reflect.Int:
		return "System.Int32"
	case reflect.Slice:
		return "System.Collections.Immutable.ImmutableList`1[" + csReflectType(t.Elem()) + "]"
	case reflect.Map:
		keyType := csReflectType(t.Key())
		elemType := csReflectType(t.Elem())
		return "System.Collections.Immutable.ImmutableDictionary`2[" + keyType + "][" + elemType + "]"
	}
	return "System.String"
}

// objectTypeResponse 通过反射生成对象类型的 TypeResponse。
func objectTypeResponse(t reflect.Type, baseType string) *typeResponse {
	body := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		jsonName := sf.Name
		if tag := sf.Tag.Get("json"); tag != "" {
			jsonName = strings.Split(tag, ",")[0]
		}
		body[jsonName] = csFieldType(sf)
	}
	return &typeResponse{
		Body: body,
		Properties: typeProperties{
			BaseType: baseType,
		},
	}
}

// getTypeInfo 返回给定 C# 类型名的 TypeResponse。
// 已知枚举/配置类型返回精确结构；未知类型（含 ASF-ui 递归解析时可能产生的容器/子类型名）返回空对象结构，
// 避免前端配置编辑器因 400 卡在加载中（原版 TypeController 对容器类型同样返回空 Body + BaseType System.Object）。
func getTypeInfo(typeName string) (*typeResponse, bool) {
	if info, ok := csEnumTypes[typeName]; ok {
		return &typeResponse{
			Body: info.Values,
			Properties: typeProperties{
				BaseType:         "System.Enum",
				CustomAttributes: info.customAttributes(),
				UnderlyingType:   info.Underlying,
			},
		}, true
	}
	switch typeName {
	case "ArchiSteamFarm.Steam.Storage.BotConfig":
		return objectTypeResponse(reflect.TypeOf(config.BotConfig{}), "System.Object"), true
	case "ArchiSteamFarm.Storage.GlobalConfig":
		return objectTypeResponse(reflect.TypeOf(config.GlobalConfig{}), "System.Object"), true
	}
	// 宽松兜底：未知类型按空对象处理
	return &typeResponse{
		Body:       map[string]string{},
		Properties: typeProperties{BaseType: "System.Object"},
	}, true
}

// getStructure 返回给定 C# 类型名的默认实例；未知类型返回空对象（对应容器类型的空结构）。
func getStructure(typeName string) (any, bool) {
	switch typeName {
	case "ArchiSteamFarm.Steam.Storage.BotConfig":
		return config.DefaultBotConfig(), true
	case "ArchiSteamFarm.Storage.GlobalConfig":
		return config.Defaults(), true
	}
	return map[string]any{}, true
}
