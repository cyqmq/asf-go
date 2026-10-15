package bot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/logging"
)

// newCommandTestBot 构造一个可用于命令测试的 Bot 实例。
func newCommandTestBot(t *testing.T) *Bot {
	t.Helper()
	cfg := config.DefaultBotConfig()
	cfg.SteamUserPermissions = map[string]config.Access{"76561198000000001": config.AccessMaster}
	dir := t.TempDir()
	log, err := logging.New("Bot-test", filepath.Join(dir, "test.log"), logging.Info)
	if err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	return &Bot{
		Name:         "testbot",
		Config:       cfg,
		Log:          log,
		CommandPrefix: "!",
		configDir:    dir,
		OwnerSteamID: 76561198000000001,
	}
}

func TestCommandHelp(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!help")
	if !strings.Contains(reply, "2fa") || !strings.Contains(reply, "mafile") ||
		!strings.Contains(reply, "password") || !strings.Contains(reply, "batch") {
		t.Errorf("help 文本缺少新命令: %q", reply)
	}
}

func TestCommand2FAWithoutAuthenticator(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!2fa")
	if !strings.Contains(reply, "2FA 代码失败") {
		t.Errorf("未配置验证器时 !2fa 应返回错误，实际: %q", reply)
	}
}

func TestCommandPassword(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!password newsecret")
	if !strings.Contains(reply, "已更新") {
		t.Fatalf("!password 应返回成功，实际: %q", reply)
	}
	// 验证配置已持久化
	cfg, err := config.LoadBotConfig(filepath.Join(b.configDir, b.Name+".json"))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.SteamPassword != "newsecret" {
		t.Errorf("SteamPassword = %q，期望 newsecret", cfg.SteamPassword)
	}
}

func TestCommandPasswordEmpty(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!password")
	if !strings.Contains(reply, "用法") {
		t.Errorf("空密码应提示用法，实际: %q", reply)
	}
}

func TestCommandKeysEmpty(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!keys")
	if !strings.Contains(reply, "为空") {
		t.Errorf("无 BGR 文件时 !keys 应提示为空，实际: %q", reply)
	}
}

func TestCommandKeysWithFile(t *testing.T) {
	b := newCommandTestBot(t)
	bgrPath := filepath.Join(b.configDir, b.Name+".games-to-redeem-in-background")
	if err := os.WriteFile(bgrPath, []byte("AAAAA-BBBBB-CCCCC\nDDDDD-EEEEE-FFFFF"), 0o600); err != nil {
		t.Fatal(err)
	}
	reply := b.handleCommandText(76561198000000001, "!keys")
	if !strings.Contains(reply, "2") || !strings.Contains(reply, "AAAAA-BBBBB-CCCCC") {
		t.Errorf("!keys 应列出密钥，实际: %q", reply)
	}
}

func TestCommandBatchEmpty(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!batch")
	if !strings.Contains(reply, "用法") {
		t.Errorf("!batch 无参数应提示用法，实际: %q", reply)
	}
}

func TestCommandPermissionDenied(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000002, "!password x")
	if !strings.Contains(reply, "权限不足") {
		t.Errorf("无权限用户调用 !password 应被拒绝，实际: %q", reply)
	}
}
func TestMatchableType(t *testing.T) {
	matchable := []config.AssetType{config.AssetTypeTradingCard, config.AssetTypeFoilTradingCard}
	cases := []struct {
		itemType string
		expected bool
	}{
		{"Trading Card", true},
		{"Foil Trading Card", true},
		{"Booster Pack", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := matchableType(matchable, tc.itemType); got != tc.expected {
			t.Errorf("matchableType(%q) = %v, 期望 %v", tc.itemType, got, tc.expected)
		}
	}
}

func TestFairExchange(t *testing.T) {
	matchable := []config.AssetType{config.AssetTypeTradingCard}
	card := assetTypeKey{appID: 753, itemType: "TradingCard"}

	cases := []struct {
		name    string
		give    map[assetTypeKey]uint32
		receive map[assetTypeKey]uint32
		expected bool
	}{
		{"公平交换", map[assetTypeKey]uint32{card: 1}, map[assetTypeKey]uint32{card: 1}, true},
		{"对方多给", map[assetTypeKey]uint32{card: 1}, map[assetTypeKey]uint32{card: 2}, true},
		{"我方多给", map[assetTypeKey]uint32{card: 2}, map[assetTypeKey]uint32{card: 1}, false},
		{"类型不可匹配", map[assetTypeKey]uint32{{appID: 753, itemType: "BoosterPack"}: 1}, map[assetTypeKey]uint32{{appID: 753, itemType: "BoosterPack"}: 1}, false},
		{"对方缺少类型", map[assetTypeKey]uint32{card: 1}, map[assetTypeKey]uint32{{appID: 753, itemType: "FoilTradingCard"}: 1}, false},
	}
	for _, tc := range cases {
		if got := fairExchange(tc.give, tc.receive, matchable); got != tc.expected {
			t.Errorf("%s: fairExchange = %v, 期望 %v", tc.name, got, tc.expected)
		}
	}
}
