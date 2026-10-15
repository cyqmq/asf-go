// Package config 移植自 ArchiSteamFarm.Storage.GlobalConfig（C#）
// 对应 ASF.json 全局配置模型。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GlobalConfig 对应 C# 的 GlobalConfig 类，使用 PascalCase JSON 以兼容 ASF.json。
type GlobalConfig struct {
	AutoRestart               bool     `json:"AutoRestart"`
	Blacklist                 []uint   `json:"Blacklist"`
	CommandPrefix             string   `json:"CommandPrefix"`
	ConfirmationsLimiterDelay uint8    `json:"ConfirmationsLimiterDelay"`
	ConnectionTimeout         uint8    `json:"ConnectionTimeout"`
	CurrentCulture            string   `json:"CurrentCulture"`
	Debug                     bool     `json:"Debug"`
	DefaultBot                string   `json:"DefaultBot"`
	FarmingDelay              uint8    `json:"FarmingDelay"`
	FilterBadBots             bool     `json:"FilterBadBots"`
	GiftsLimiterDelay         uint8    `json:"GiftsLimiterDelay"`
	Headless                  bool     `json:"Headless"`
	IPC                       bool     `json:"IPC"`
	IPCPassword               string   `json:"IPCPassword"`
	IPCPasswordFormat         string   `json:"IPCPasswordFormat"`
	IdleFarmingPeriod         uint8    `json:"IdleFarmingPeriod"`
	InventoryLimiterDelay     uint8    `json:"InventoryLimiterDelay"`
	LicenseID                 string   `json:"LicenseID"`
	LoginLimiterDelay         uint8    `json:"LoginLimiterDelay"`
	MaxFarmingTime            uint8    `json:"MaxFarmingTime"`
	MaxTradeHoldDuration      uint8    `json:"MaxTradeHoldDuration"`
	MinFarmingDelayAfterBlock uint8    `json:"MinFarmingDelayAfterBlock"`
	OptimizationMode          string   `json:"OptimizationMode"`
	PluginsUpdateList         []string `json:"PluginsUpdateList"`
	PluginsUpdateMode         string   `json:"PluginsUpdateMode"`
	PluginsDir                string   `json:"PluginsDir"`
	ShutdownIfPossible        bool     `json:"ShutdownIfPossible"`
	SteamMessagePrefix        string   `json:"SteamMessagePrefix"`
	SteamOwnerID              uint64   `json:"SteamOwnerID"`
	SteamProtocols            string   `json:"SteamProtocols"`
	UpdateChannel             string   `json:"UpdateChannel"`
	UpdatePeriod              uint8    `json:"UpdatePeriod"`
	WebLimiterDelay           uint16   `json:"WebLimiterDelay"`
	WebProxyText              string   `json:"WebProxy"`
	WebProxyUsername          string   `json:"WebProxyUsername"`
	SteamWebApiKey            string   `json:"SteamWebApiKey"`
	IPCOrigins                []string `json:"IPCOrigins,omitempty"`
}

// Defaults 返回 ASF 默认全局配置。
func Defaults() *GlobalConfig {
	return &GlobalConfig{
		AutoRestart:               true,
		CommandPrefix:             "!",
		ConfirmationsLimiterDelay: 10,
		ConnectionTimeout:         90,
		FarmingDelay:              15,
		FilterBadBots:             true,
		GiftsLimiterDelay:         1,
		IPC:                       true,
		IPCPasswordFormat:         "PlainText",
		IdleFarmingPeriod:         8,
		InventoryLimiterDelay:     4,
		LoginLimiterDelay:         10,
		MaxFarmingTime:            10,
		MaxTradeHoldDuration:      15,
		MinFarmingDelayAfterBlock: 60,
		OptimizationMode:          "MaxPerformance",
		PluginsUpdateList:         []string{},
		PluginsUpdateMode:         "Whitelist",
		PluginsDir:                "plugins",
		SteamMessagePrefix:        "/me ",
		SteamProtocols:            "All",
		UpdateChannel:             "Stable",
		UpdatePeriod:              24,
		WebLimiterDelay:           300,
	}
}

// Load 从指定路径加载 ASF.json；文件不存在时返回默认配置。
func Load(path string) (*GlobalConfig, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败 %s: %w", path, err)
	}
	return cfg, nil
}

// Save 将配置写入指定路径（先创建目录）。
func (c *GlobalConfig) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
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
