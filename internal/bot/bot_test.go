package bot

import (
	"testing"

	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/inventory"
)

func TestShouldAcceptFriend(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.SteamUserPermissions = map[string]config.Access{"76561198000000001": config.AccessMaster}
	cfg.TradingAllowableSteamIDs = []uint64{76561198000000002}
	b := &Bot{Config: cfg}

	cases := []struct {
		name     string
		steamID  uint64
		expected bool
	}{
		{"权限列表中的用户", 76561198000000001, true},
		{"交易允许列表中的用户", 76561198000000002, true},
		{"普通用户", 76561198000000003, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.shouldAcceptFriend(tc.steamID); got != tc.expected {
				t.Errorf("shouldAcceptFriend(%d) = %v, 期望 %v", tc.steamID, got, tc.expected)
			}
		})
	}
}

func TestShouldAcceptFriendWithAutoAccept(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.AcceptFriendRequests = true
	b := &Bot{Config: cfg}
	if !b.shouldAcceptFriend(76561198000000009) {
		t.Error("AcceptFriendRequests=true 时应接受任意用户")
	}
}

func TestIsLootable(t *testing.T) {
	cases := []struct {
		typ      string
		expected bool
	}{
		{"Trading Card", true},
		{"Foil Trading Card", true},
		{"Booster Pack", true},
		{"Steam Gems", false},
		{"Gift", false},
		{"", false},
	}

	for _, tc := range cases {
		item := inventory.Item{Type: tc.typ}
		if got := isLootable(item); got != tc.expected {
			t.Errorf("isLootable(%q) = %v, 期望 %v", tc.typ, got, tc.expected)
		}
	}
}

func TestLootTargets(t *testing.T) {
	cfg := config.DefaultBotConfig()

	t.Run("允许列表优先", func(t *testing.T) {
		cfg.TradingAllowableSteamIDs = []uint64{76561198000000001, 76561198000000002}
		b := &Bot{Config: cfg, OwnerSteamID: 76561198000000000}
		targets := b.lootTargets()
		if len(targets) != 2 || targets[0] != 76561198000000001 {
			t.Errorf("应返回允许列表，得到 %v", targets)
		}
	})

	t.Run("允许列表为空时发给 Owner", func(t *testing.T) {
		cfg.TradingAllowableSteamIDs = []uint64{}
		b := &Bot{Config: cfg, OwnerSteamID: 76561198000000000}
		targets := b.lootTargets()
		if len(targets) != 1 || targets[0] != 76561198000000000 {
			t.Errorf("应返回 Owner，得到 %v", targets)
		}
	})

	t.Run("无目标返回空", func(t *testing.T) {
		cfg.TradingAllowableSteamIDs = []uint64{}
		b := &Bot{Config: cfg, OwnerSteamID: 0}
		if targets := b.lootTargets(); targets != nil {
			t.Errorf("应返回 nil，得到 %v", targets)
		}
	})
}

func TestCheckAppBetaNotLoggedIn(t *testing.T) {
	b := &Bot{Name: "testbot"}
	if _, err := b.CheckAppBeta(730, "password"); err == nil {
		t.Error("未登录时应返回错误")
	}
}

func TestOwnsValidation(t *testing.T) {
	t.Run("未登录", func(t *testing.T) {
		b := &Bot{Name: "testbot", SteamAPIKey: "key"}
		if _, err := b.Owns([]string{"730"}); err == nil {
			t.Error("未登录时应返回错误")
		}
	})

	t.Run("未配置 API Key", func(t *testing.T) {
		b := &Bot{Name: "testbot"}
		if _, err := b.Owns([]string{"730"}); err == nil {
			t.Error("未配置 SteamWebApiKey 时应返回错误")
		}
	})
}

func TestRedeemNotLoggedIn(t *testing.T) {
	b := &Bot{Name: "testbot"}
	if _, err := b.Redeem([]string{"AAAAA-BBBBB-CCCCC"}); err == nil {
		t.Error("未登录时应返回错误")
	}
}

func TestCheckAccess(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.SteamUserPermissions = map[string]config.Access{
		"76561198000000001": config.AccessMaster,
		"76561198000000002": config.AccessOperator,
	}
	b := &Bot{Name: "testbot", Config: cfg, OwnerSteamID: 76561198000000000}

	cases := []struct {
		name     string
		chatter  uint64
		min      config.Access
		expected bool
	}{
		{"Owner 总是通过", 76561198000000000, config.AccessMaster, true},
		{"Master 用户通过 Master 要求", 76561198000000001, config.AccessMaster, true},
		{"Operator 用户通过 Operator 要求", 76561198000000002, config.AccessOperator, true},
		{"Operator 用户不通过 Master 要求", 76561198000000002, config.AccessMaster, false},
		{"普通用户不通过 Operator 要求", 76561198000000003, config.AccessOperator, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.checkAccess(tc.chatter, tc.min); got != tc.expected {
				t.Errorf("checkAccess(%d, %s) = %v, 期望 %v", tc.chatter, tc.min, got, tc.expected)
			}
		})
	}
}

func TestCheckAccessBlacklist(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.SteamUserPermissions = map[string]config.Access{"76561198000000001": config.AccessMaster}
	b := &Bot{Name: "testbot", Config: cfg, OwnerSteamID: 76561198000000000, Blacklist: []uint64{76561198000000001}}

	if b.checkAccess(76561198000000001, config.AccessMaster) {
		t.Error("黑名单用户即使有 Master 权限也应被拒绝")
	}
}

func TestPointsBalanceValidation(t *testing.T) {
	t.Run("未登录", func(t *testing.T) {
		b := &Bot{Name: "testbot", SteamAPIKey: "key"}
		if _, err := b.PointsBalance(); err == nil {
			t.Error("未登录时应返回错误")
		}
	})

	t.Run("未配置 API Key", func(t *testing.T) {
		b := &Bot{Name: "testbot"}
		if _, err := b.PointsBalance(); err == nil {
			t.Error("未配置 SteamWebApiKey 时应返回错误")
		}
	})
}