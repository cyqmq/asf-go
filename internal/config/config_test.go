package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestGlobalConfigLoadDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.AutoRestart != true {
		t.Errorf("AutoRestart 默认值应为 true，得到 %v", cfg.AutoRestart)
	}
	if cfg.CommandPrefix != "!" {
		t.Errorf("CommandPrefix 默认值应为 !，得到 %q", cfg.CommandPrefix)
	}
	if cfg.ConnectionTimeout != 90 {
		t.Errorf("ConnectionTimeout 默认值应为 90，得到 %d", cfg.ConnectionTimeout)
	}
	if cfg.UpdatePeriod != 24 {
		t.Errorf("UpdatePeriod 默认值应为 24，得到 %d", cfg.UpdatePeriod)
	}
}

func TestGlobalConfigJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ASF.json")

	cfg := Defaults()
	cfg.SteamOwnerID = 76561198000000000
	cfg.DefaultBot = "mybot"
	cfg.Blacklist = []uint{1, 2, 3}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if loaded.SteamOwnerID != cfg.SteamOwnerID {
		t.Errorf("SteamOwnerID 不一致: %d != %d", loaded.SteamOwnerID, cfg.SteamOwnerID)
	}
	if loaded.DefaultBot != cfg.DefaultBot {
		t.Errorf("DefaultBot 不一致: %q != %q", loaded.DefaultBot, cfg.DefaultBot)
	}
	if len(loaded.Blacklist) != 3 {
		t.Errorf("Blacklist 长度不一致: %d != 3", len(loaded.Blacklist))
	}
}

func TestBotConfigJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bot.json")

	cfg := DefaultBotConfig()
	cfg.SteamLogin = "login"
	cfg.Enabled = true
	cfg.SteamUserPermissions = map[string]Access{"76561198000000000": AccessMaster}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	loaded, err := LoadBotConfig(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if loaded.SteamLogin != "login" {
		t.Errorf("SteamLogin 不一致: %q", loaded.SteamLogin)
	}
	if loaded.Enabled != true {
		t.Errorf("Enabled 不一致")
	}
	if loaded.SteamUserPermissions["76561198000000000"] != AccessMaster {
		t.Errorf("权限不一致: %v", loaded.SteamUserPermissions)
	}
}

func TestGlobalConfigPascalCaseJSON(t *testing.T) {
	// 验证 JSON 字段名使用 PascalCase，与 ASF 原版配置兼容。
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"AutoRestart":true,"SteamOwnerID":123}`), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["AutoRestart"]; !ok {
		t.Error("缺少 AutoRestart 字段")
	}
}

func TestTradingPreferencesJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bot.json")

	cfg := DefaultBotConfig()
	cfg.Trading = true
	cfg.TradingPreferences = TradingPreferencesAcceptAll | TradingPreferencesSteamlvlLimit
	cfg.TradingPeriod = 2
	cfg.TradingSteamLevel = 10
	cfg.TradingAllowableSteamIDs = []uint64{76561198000000000}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	loaded, err := LoadBotConfig(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if loaded.Trading != true {
		t.Errorf("Trading 不一致: %v", loaded.Trading)
	}
	if loaded.TradingPreferences != cfg.TradingPreferences {
		t.Errorf("TradingPreferences 不一致: %d != %d", loaded.TradingPreferences, cfg.TradingPreferences)
	}
	if !loaded.TradingPreferences.Has(TradingPreferencesAcceptAll) {
		t.Error("应包含 AcceptAll 标志")
	}
	if loaded.TradingPreferences.Has(TradingPreferencesPassive) {
		t.Error("不应包含 Passive 标志")
	}
	if loaded.TradingPeriod != 2 {
		t.Errorf("TradingPeriod 不一致: %d", loaded.TradingPeriod)
	}
	if loaded.TradingSteamLevel != 10 {
		t.Errorf("TradingSteamLevel 不一致: %d", loaded.TradingSteamLevel)
	}
	if len(loaded.TradingAllowableSteamIDs) != 1 {
		t.Errorf("TradingAllowableSteamIDs 不一致: %v", loaded.TradingAllowableSteamIDs)
	}
}

func TestTradingPreferencesBitmask(t *testing.T) {
	v := TradingPreferencesAcceptGifts | TradingPreferencesSendTrades
	if !v.Has(TradingPreferencesAcceptGifts) {
		t.Error("应包含 AcceptGifts")
	}
	if !v.Has(TradingPreferencesSendTrades) {
		t.Error("应包含 SendTrades")
	}
	if v.Has(TradingPreferencesPassive) {
		t.Error("不应包含 Passive")
	}
}

func TestBotBehaviourJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bot.json")

	cfg := DefaultBotConfig()
	cfg.BotBehaviour = BotBehaviourRejectInvalidFriendRequests | BotBehaviourRejectInvalidGroupInvites
	cfg.AcceptFriendRequests = true

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	loaded, err := LoadBotConfig(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if loaded.BotBehaviour != cfg.BotBehaviour {
		t.Errorf("BotBehaviour 不一致: %d != %d", loaded.BotBehaviour, cfg.BotBehaviour)
	}
	if !loaded.BotBehaviour.Has(BotBehaviourRejectInvalidFriendRequests) {
		t.Error("应包含 RejectInvalidFriendRequests")
	}
	if loaded.BotBehaviour.Has(BotBehaviourDismissInventoryNotifications) {
		t.Error("不应包含 DismissInventoryNotifications")
	}
	if loaded.AcceptFriendRequests != true {
		t.Error("AcceptFriendRequests 应为 true")
	}
}

func TestBotBehaviourBitmask(t *testing.T) {
	v := BotBehaviourRejectInvalidFriendRequests | BotBehaviourMarkBotMessagesAsRead
	if !v.Has(BotBehaviourRejectInvalidFriendRequests) {
		t.Error("应包含 RejectInvalidFriendRequests")
	}
	if !v.Has(BotBehaviourMarkBotMessagesAsRead) {
		t.Error("应包含 MarkBotMessagesAsRead")
	}
	if v.Has(BotBehaviourRejectInvalidTrades) {
		t.Error("不应包含 RejectInvalidTrades")
	}
}

func TestAccessRank(t *testing.T) {
	cases := []struct {
		access Access
		rank   int
	}{
		{AccessNone, 0},
		{AccessFamilySharing, 1},
		{AccessOperator, 2},
		{AccessMaster, 3},
	}
	for _, tc := range cases {
		if got := tc.access.Rank(); got != tc.rank {
			t.Errorf("AccessRank(%s) = %d, 期望 %d", tc.access, got, tc.rank)
		}
	}
}
