// Package config 移植自 ArchiSteamFarm.Steam.Storage.BotConfig（C#）
// 对应每个 Bot 的 <BotName>.json 配置模型。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	asfcrypto "github.com/asf-go/asf/internal/crypto"
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

// FlexString 是兼容数字/字符串/布尔值的宽松字符串类型。
// 原版 ASF 的位标志枚举（如 EFarmingPreferences）序列化为数字，我们的配置模型用字符串表示，
// 使用 FlexString 可同时接受两种形式而不丢失值。
type FlexString string

// UnmarshalJSON 接受字符串、数字、布尔值并统一存为字符串。
func (s *FlexString) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = FlexString(str)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		*s = FlexString(num.String())
		return nil
	}
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*s = FlexString(fmt.Sprintf("%v", b))
		return nil
	}
	return nil
}

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
	FarmingPreferences           FlexString         `json:"FarmingPreferences"`
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
	RedeemingPreferences         FlexString         `json:"RedeemingPreferences"`
	RemoteCommunication          FlexString         `json:"RemoteCommunication"`
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

// GetDecryptedSteamPassword 返回解密后的 Steam 密码。
// 当 PasswordFormat=AES 且值带 encrypted: 前缀时，用 ASF_CRYPTKEY 解密密文；否则原样返回（兼容明文配置）。
func (c *BotConfig) GetDecryptedSteamPassword() string {
	if c.PasswordFormat == "AES" && strings.HasPrefix(c.SteamPassword, "encrypted:") {
		if decrypted, err := asfcrypto.DecryptAES(strings.TrimPrefix(c.SteamPassword, "encrypted:")); err == nil {
			return decrypted
		}
	}
	return c.SteamPassword
}

// SetSteamPassword 设置 Steam 密码；启用 ASF_CRYPTKEY 时加密存储并标记 PasswordFormat=AES，否则明文存储。
func (c *BotConfig) SetSteamPassword(plaintext string) {
	if asfcrypto.EncryptionEnabled() {
		if encrypted, err := asfcrypto.EncryptAES(plaintext); err == nil {
			c.SteamPassword = "encrypted:" + encrypted
			c.PasswordFormat = "AES"
			return
		}
	}
	c.SteamPassword = plaintext
	c.PasswordFormat = "PlainText"
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
