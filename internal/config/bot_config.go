// Package config 移植自 ArchiSteamFarm.Steam.Storage.BotConfig（C#）
// 对应每个 Bot 的 <BotName>.json 配置模型。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AssetType 对应 C# EAssetType 枚举。
type AssetType string

const (
	AssetTypeTradingCard     AssetType = "TradingCard"
	AssetTypeFoilTradingCard AssetType = "FoilTradingCard"
	AssetTypeBoosterPack     AssetType = "BoosterPack"
)

// PersonaState 对应 C# EPersonaState 枚举。
type PersonaState string

const (
	PersonaStateOffline       PersonaState = "Offline"
	PersonaStateOnline        PersonaState = "Online"
	PersonaStateAway          PersonaState = "Away"
	PersonaStateLookingToPlay PersonaState = "LookingToPlay"
	PersonaStateSnooze        PersonaState = "Snooze"
)

// Access 对应 C# EAccess 枚举。
type Access string

const (
	AccessNone          Access = "None"
	AccessFamilySharing Access = "FamilySharing"
	AccessOperator      Access = "Operator"
	AccessMaster        Access = "Master"
)

// Rank 返回访问权限的等级（用于比较）。
func (a Access) Rank() int {
	switch a {
	case AccessMaster:
		return 3
	case AccessOperator:
		return 2
	case AccessFamilySharing:
		return 1
	default:
		return 0
	}
}

// TradingPreferences 对应 C# ETradingPreferences 枚举（位掩码）。
type TradingPreferences uint16

const (
	TradingPreferencesNone              TradingPreferences = 0
	TradingPreferencesAcceptAll         TradingPreferences = 1
	TradingPreferencesSteamlvlLimit     TradingPreferences = 2
	TradingPreferencesMatchMakingOnly   TradingPreferences = 4
	TradingPreferencesDontPreferItems   TradingPreferences = 8
	TradingPreferencesPassive           TradingPreferences = 16
	TradingPreferencesAcceptGifts       TradingPreferences = 32
	TradingPreferencesSendTradePeriod   TradingPreferences = 64
	TradingPreferencesSendTrades        TradingPreferences = 128
	TradingPreferencesSteamTradeMatcher TradingPreferences = 256
)

// Has 判断位掩码是否包含指定标志。
func (t TradingPreferences) Has(flag TradingPreferences) bool {
	return t&flag != 0
}

// BotBehaviour 对应 C# EBotBehaviour 枚举（位掩码）。
type BotBehaviour uint32

const (
	BotBehaviourNone                          BotBehaviour = 0
	BotBehaviourRejectInvalidFriendRequests   BotBehaviour = 1 << 0
	BotBehaviourRejectInvalidTrades           BotBehaviour = 1 << 1
	BotBehaviourRejectInvalidGroupInvites     BotBehaviour = 1 << 2
	BotBehaviourDismissInventoryNotifications BotBehaviour = 1 << 3
	BotBehaviourMarkReceivedMessagesAsRead    BotBehaviour = 1 << 4
	BotBehaviourMarkBotMessagesAsRead         BotBehaviour = 1 << 5
	BotBehaviourRejectInvalidGiftRedeems      BotBehaviour = 1 << 6
)

// Has 判断位掩码是否包含指定标志。
func (b BotBehaviour) Has(flag BotBehaviour) bool {
	return b&flag != 0
}

// BotConfig 对应 C# BotConfig 类。
type BotConfig struct {
	AcceptFriendRequests         bool               `json:"AcceptFriendRequests"`
	AcceptGifts                  bool               `json:"AcceptGifts"`
	BotBehaviour                 BotBehaviour       `json:"BotBehaviour"`
	CompleteTypesToSend          []AssetType        `json:"CompleteTypesToSend"`
	CustomGamePlayedWhileFarming string             `json:"CustomGamePlayedWhileFarming"`
	CustomGamePlayedWhileIdle    string             `json:"CustomGamePlayedWhileIdle"`
	Enabled                      bool               `json:"Enabled"`
	FarmingOrders                []string           `json:"FarmingOrders"`
	FarmingPreferences           string             `json:"FarmingPreferences"`
	FarmingBlacklist             []uint32           `json:"FarmingBlacklist"`
	FarmingQueue                 []uint32           `json:"FarmingQueue"`
	GamesPlayedWhileIdle         []uint             `json:"GamesPlayedWhileIdle"`
	GamingDeviceType             string             `json:"GamingDeviceType"`
	HoursUntilCardDrops          uint8              `json:"HoursUntilCardDrops"`
	LootableTypes                []AssetType        `json:"LootableTypes"`
	MachineName                  string             `json:"MachineName"`
	MatchableTypes               []AssetType        `json:"MatchableTypes"`
	OnlineFlags                  uint64             `json:"OnlineFlags"`
	OnlineStatus                 PersonaState       `json:"OnlineStatus"`
	PasswordFormat               string             `json:"PasswordFormat"`
	RedeemingPreferences         string             `json:"RedeemingPreferences"`
	RemoteCommunication          string             `json:"RemoteCommunication"`
	SendTradePeriod              uint8              `json:"SendTradePeriod"`
	SteamLogin                   string             `json:"SteamLogin"`
	SteamMasterClanID            uint64             `json:"SteamMasterClanID"`
	SteamParentalCode            string             `json:"SteamParentalCode"`
	SteamPassword                string             `json:"SteamPassword"`
	SteamTradeToken              string             `json:"SteamTradeToken"`
	SteamUserPermissions         map[string]Access  `json:"SteamUserPermissions"`
	TradeCheckPeriod             uint8              `json:"TradeCheckPeriod"`
	Trading                      bool               `json:"Trading"`
	TradingAllowableSteamIDs     []uint64           `json:"TradingAllowableSteamIDs"`
	TradingBlacklist             []uint64           `json:"TradingBlacklist"`
	TradingPeriod                uint16             `json:"TradingPeriod"`
	TradingPreferences           TradingPreferences `json:"TradingPreferences"`
	TradingSteamLevel            uint8              `json:"TradingSteamLevel"`
	TransferableTypes            []AssetType        `json:"TransferableTypes"`
	UseLoginKeys                 bool               `json:"UseLoginKeys"`
	UserInterfaceMode            string             `json:"UserInterfaceMode"`
	WebProxyText                 string             `json:"WebProxy"`
	WebProxyUsername             string             `json:"WebProxyUsername"`
}

// DefaultBotConfig 返回默认 Bot 配置。
func DefaultBotConfig() *BotConfig {
	return &BotConfig{
		AcceptFriendRequests:     false,
		BotBehaviour:             BotBehaviourNone,
		GamingDeviceType:         "StandardPC",
		HoursUntilCardDrops:      3,
		OnlineStatus:             PersonaStateOnline,
		PasswordFormat:           "PlainText",
		RemoteCommunication:      "All",
		SteamMasterClanID:        0,
		TradeCheckPeriod:         60,
		Trading:                  true,
		TradingAllowableSteamIDs: []uint64{},
		TradingBlacklist:         []uint64{},
		TradingPeriod:            1,
		TradingPreferences:       TradingPreferencesNone,
		TradingSteamLevel:        5,
		UseLoginKeys:             true,
		UserInterfaceMode:        "VGUI",
		LootableTypes:            []AssetType{AssetTypeBoosterPack, AssetTypeFoilTradingCard, AssetTypeTradingCard},
		MatchableTypes:           []AssetType{AssetTypeTradingCard},
		TransferableTypes:        []AssetType{AssetTypeBoosterPack, AssetTypeFoilTradingCard, AssetTypeTradingCard},
		SteamUserPermissions:     map[string]Access{},
		CompleteTypesToSend:      []AssetType{},
		FarmingOrders:            []string{},
		FarmingBlacklist:         []uint32{},
		FarmingQueue:             []uint32{},
		GamesPlayedWhileIdle:     []uint{},
	}
}

// LoadBotConfig 从指定路径加载 <BotName>.json；文件不存在时返回默认配置。
func LoadBotConfig(path string) (*BotConfig, error) {
	cfg := DefaultBotConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取 Bot 配置失败: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析 Bot 配置失败 %s: %w", path, err)
	}
	return cfg, nil
}

// Save 将 Bot 配置写入指定路径。
func (c *BotConfig) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}
