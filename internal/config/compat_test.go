package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadOriginalBotConfig 用原版 ASF 风格的 BotConfig JSON 验证字段兼容性。
func TestLoadOriginalBotConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testbot.json")
	json := `{
		"AcceptFriendRequests": false,
		"AcceptGifts": false,
		"BotBehaviour": 0,
		"CompleteTypesToSend": ["BoosterPack", "TradingCard"],
		"CustomGamePlayedWhileFarming": null,
		"CustomGamePlayedWhileIdle": null,
		"Enabled": true,
		"FarmingOrders": ["Unordered"],
		"FarmingPreferences": 0,
		"GamesPlayedWhileIdle": [1, 2],
		"GamingDeviceType": "StandardPC",
		"HoursUntilCardDrops": 3,
		"LootableTypes": ["BoosterPack", "TradingCard"],
		"MatchableTypes": ["TradingCard"],
		"OnlineStatus": "Online",
		"PasswordFormat": "PlainText",
		"SteamLogin": "testlogin",
		"SteamPassword": "testpass",
		"SteamUserPermissions": {"76561198000000000": "Master"},
		"Trading": true,
		"UseLoginKeys": true
	}`
	if err := os.WriteFile(path, []byte(json), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadBotConfig(path)
	if err != nil {
		t.Fatalf("加载原版 BotConfig 失败: %v", err)
	}
	if !cfg.Enabled {
		t.Error("Enabled 应为 true")
	}
	if cfg.SteamLogin != "testlogin" || cfg.SteamPassword != "testpass" {
		t.Errorf("Steam 凭据未正确解析: %q / %q", cfg.SteamLogin, cfg.SteamPassword)
	}
	if cfg.GamingDeviceType != "StandardPC" || cfg.OnlineStatus != "Online" {
		t.Errorf("枚举字段未正确解析: %q / %q", cfg.GamingDeviceType, cfg.OnlineStatus)
	}
	if len(cfg.LootableTypes) != 2 {
		t.Errorf("LootableTypes 应包含 2 项，实际 %d", len(cfg.LootableTypes))
	}
	if cfg.SteamUserPermissions["76561198000000000"] != AccessMaster {
		t.Errorf("SteamUserPermissions 未正确解析")
	}
}

// TestLoadOriginalGlobalConfig 用原版 ASF 风格的 GlobalConfig JSON 验证兼容性（含未知字段）。
func TestLoadOriginalGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ASF.json")
	json := `{
		"AutoRestart": true,
		"Blacklist": [76561198000000000],
		"CommandPrefix": "!",
		"ConnectionTimeout": 90,
		"CurrentCulture": "en-US",
		"Debug": false,
		"FarmingDelay": 10,
		"Headless": false,
		"IPC": true,
		"IPCPassword": "testpass",
		"IPCPasswordFormat": "PlainText",
		"MaxFarmingTime": 0,
		"SteamOwnerID": 76561198000000000,
		"UpdateChannel": "Stable",
		"WebProxy": ""
	}`
	if err := os.WriteFile(path, []byte(json), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载原版 GlobalConfig 失败: %v", err)
	}
	if !cfg.IPC || cfg.IPCPassword != "testpass" {
		t.Errorf("IPC 配置未正确解析: IPC=%v", cfg.IPC)
	}
	if len(cfg.Blacklist) != 1 {
		t.Errorf("Blacklist 应包含 1 项，实际 %d", len(cfg.Blacklist))
	}
	if cfg.CommandPrefix != "!" {
		t.Errorf("CommandPrefix = %q", cfg.CommandPrefix)
	}
}

func TestGlobalConfigWebProxyPasswordField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ASF.json")
	json := `{
		"WebProxy": "http://127.0.0.1:8080",
		"WebProxyUsername": "user",
		"WebProxyPassword": "pass"
	}`
	if err := os.WriteFile(path, []byte(json), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.WebProxyText != "http://127.0.0.1:8080" {
		t.Errorf("WebProxyText = %q", cfg.WebProxyText)
	}
	if cfg.WebProxyUsername != "user" {
		t.Errorf("WebProxyUsername = %q", cfg.WebProxyUsername)
	}
	if cfg.GetDecryptedWebProxyPassword() != "pass" {
		t.Errorf("WebProxyPassword = %q", cfg.GetDecryptedWebProxyPassword())
	}
}
