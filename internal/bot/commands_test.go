package bot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		Name:          "testbot",
		Config:        cfg,
		Log:           log,
		CommandPrefix: "!",
		configDir:     dir,
		OwnerSteamID:  76561198000000001,
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
		name     string
		give     map[assetTypeKey]uint32
		receive  map[assetTypeKey]uint32
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

func TestQRChallengeURLConcurrent(t *testing.T) {
	b := newCommandTestBot(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			_ = b.QRChallengeURL()
		}
	}()
	for i := 0; i < 1000; i++ {
		b.qrMu.Lock()
		b.qrChallengeURL = "https://s.team/q/test"
		b.qrMu.Unlock()
	}
	<-done
}

func TestStatePersistence(t *testing.T) {
	b := newCommandTestBot(t)
	games := []GameInfo{
		{AppID: 730, Name: "Counter-Strike 2", CardsRemaining: 3},
		{AppID: 440, Name: "Team Fortress 2", CardsRemaining: 1},
	}
	b.saveState(games)
	loaded := b.loadState()
	if len(loaded) != 2 {
		t.Fatalf("loadState 应返回 2 个游戏，实际 %d", len(loaded))
	}
	if loaded[0].AppID != 730 || loaded[0].CardsRemaining != 3 {
		t.Errorf("恢复的游戏数据不正确: %+v", loaded[0])
	}
}

func TestCommandPlayEmpty(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!play")
	if !strings.Contains(reply, "恢复自动挂机") {
		t.Errorf("!play 空参数应恢复挂机，实际: %q", reply)
	}
}

func TestCommandPlayNoClient(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!play 730")
	if !strings.Contains(reply, "未就绪") {
		t.Errorf("无 steamClient 时应提示未就绪，实际: %q", reply)
	}
}

func TestCommandInputEmpty(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!input")
	if !strings.Contains(reply, "用法") {
		t.Errorf("!input 空参数应提示用法，实际: %q", reply)
	}
}

func TestCommandInputNoClient(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!input 12345")
	if !strings.Contains(reply, "未就绪") {
		t.Errorf("无 steamClient 时应提示未就绪，实际: %q", reply)
	}
}

func TestCommand2FAOkWithoutAuthenticator(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!2faok")
	if !strings.Contains(reply, "获取确认失败") {
		t.Errorf("未配置验证器时 !2faok 应报错，实际: %q", reply)
	}
}

func TestCommandInventoryNoClient(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!inventory")
	if !strings.Contains(reply, "查询库存失败") {
		t.Errorf("无连接时 !inventory 应报错，实际: %q", reply)
	}
}

func TestCommandBGR(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!bgr ABCDE-12345")
	if !strings.Contains(reply, "已添加") {
		t.Fatalf("!bgr 应添加成功，实际: %q", reply)
	}
	// 重复添加
	reply = b.handleCommandText(76561198000000001, "!bgr ABCDE-12345")
	if !strings.Contains(reply, "已在") {
		t.Errorf("重复添加应提示已存在，实际: %q", reply)
	}
	// 查看
	reply = b.handleCommandText(76561198000000001, "!keys")
	if !strings.Contains(reply, "ABCDE-12345") {
		t.Errorf("!keys 应包含新 key，实际: %q", reply)
	}
	// 移除
	reply = b.handleCommandText(76561198000000001, "!bgrm ABCDE-12345")
	if !strings.Contains(reply, "剩余 0 个") {
		t.Errorf("!bgrm 应移除成功，实际: %q", reply)
	}
	// 清空
	reply = b.handleCommandText(76561198000000001, "!bgr XYZ-999")
	if !strings.Contains(reply, "已添加") {
		t.Fatalf("再次添加失败: %q", reply)
	}
	reply = b.handleCommandText(76561198000000001, "!bgrclear")
	if !strings.Contains(reply, "已清空") {
		t.Errorf("!bgrclear 应清空，实际: %q", reply)
	}
}

func TestCommandHash(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!hash secret")
	if !strings.HasPrefix(reply, "SCrypt 哈希: ") {
		t.Errorf("!hash 应返回 SCrypt 哈希，实际: %q", reply)
	}
}

func TestCommandEncrypt(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!encrypt secret")
	if !strings.HasPrefix(reply, "加密结果: ") {
		t.Errorf("!encrypt 应返回加密结果，实际: %q", reply)
	}
}

func TestCommandPrivacyNotConnected(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!privacy 3 3 3 3 3 3 1")
	if !strings.Contains(reply, "尚未登录") {
		t.Errorf("未连接时 !privacy 应提示未登录，实际: %q", reply)
	}
}

func TestCommandPrivacyInvalidArgs(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!privacy 9")
	if !strings.Contains(reply, "无效") {
		t.Errorf("非法隐私值应提示无效，实际: %q", reply)
	}
}

func TestCommandReset(t *testing.T) {
	t.Skip("!reset 触发网络重连，在集成环境验证")
}

func TestCommandMAB(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!mab")
	if !strings.Contains(reply, "为空") {
		t.Fatalf("空 MAB 应提示为空，实际: %q", reply)
	}
	reply = b.handleCommandText(76561198000000001, "!mabadd 730,570,440")
	if !strings.Contains(reply, "已将") {
		t.Fatalf("!mabadd 应成功，实际: %q", reply)
	}
	reply = b.handleCommandText(76561198000000001, "!mab")
	if !strings.Contains(reply, "730") || !strings.Contains(reply, "570") || !strings.Contains(reply, "440") {
		t.Errorf("!mab 应列出全部 appid，实际: %q", reply)
	}
	reply = b.handleCommandText(76561198000000001, "!mabrm all")
	if !strings.Contains(reply, "已清空") {
		t.Errorf("!mabrm all 应清空，实际: %q", reply)
	}
	reply = b.handleCommandText(76561198000000001, "!mab")
	if !strings.Contains(reply, "为空") {
		t.Errorf("清空后 !mab 应为空，实际: %q", reply)
	}
}

func TestParseRedeemFlags(t *testing.T) {
	flags, err := ParseRedeemFlags([]string{"ff", "fawk", "fd"})
	if err != nil {
		t.Fatalf("解析 flags 失败: %v", err)
	}
	if flags&RedeemForceForwarding == 0 || flags&RedeemForceAssumeWalletKeyOnBadActivationCode == 0 || flags&RedeemForceDistributing == 0 {
		t.Errorf("flags 解析不完整: %b", flags)
	}
	if _, err := ParseRedeemFlags([]string{"bogus"}); err == nil {
		t.Errorf("未知 flag 应报错")
	}
}

func TestCommandRedeemFlags(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!redeem ABCDE-12345 ff")
	if !strings.Contains(reply, "尚未登录") {
		t.Errorf("未登录时 !redeem 应提示未登录，实际: %q", reply)
	}
	reply = b.handleCommandText(76561198000000001, "!redeem ABCDE-12345 bogus")
	if !strings.Contains(reply, "无效 flag") {
		t.Errorf("未知 flag 应报错，实际: %q", reply)
	}
}

func TestCommandFBAll(t *testing.T) {
	b := newCommandTestBot(t)
	if err := b.FarmingBlacklistAdd(730); err != nil {
		t.Fatalf("添加黑名单失败: %v", err)
	}
	reply := b.handleCommandText(76561198000000001, "!fbrm all")
	if !strings.Contains(reply, "已清空") {
		t.Fatalf("!fbrm all 应清空，实际: %q", reply)
	}
	if len(b.Config.FarmingBlacklist) != 0 {
		t.Errorf("挂机黑名单应清空，实际: %v", b.Config.FarmingBlacklist)
	}
}

func TestCommandFQAll(t *testing.T) {
	b := newCommandTestBot(t)
	if err := b.FarmingQueueAdd(570); err != nil {
		t.Fatalf("添加队列失败: %v", err)
	}
	reply := b.handleCommandText(76561198000000001, "!fqrm all")
	if !strings.Contains(reply, "已清空") {
		t.Fatalf("!fqrm all 应清空，实际: %q", reply)
	}
	if len(b.Config.FarmingQueue) != 0 {
		t.Errorf("挂机队列应清空，实际: %v", b.Config.FarmingQueue)
	}
}

func TestRemoteCommunicationAllowsSteam(t *testing.T) {
	cfg := config.DefaultBotConfig()
	cfg.RemoteCommunication = "All"
	b := &Bot{Config: cfg}
	if !b.remoteCommunicationAllowsSteam() {
		t.Errorf("All 应允许 Steam 命令")
	}
	cfg.RemoteCommunication = "SteamGroup"
	if !b.remoteCommunicationAllowsSteam() {
		t.Errorf("SteamGroup 应允许 Steam 命令")
	}
	cfg.RemoteCommunication = "None"
	if b.remoteCommunicationAllowsSteam() {
		t.Errorf("None 应禁用 Steam 命令")
	}
	cfg.RemoteCommunication = "PublicListing"
	if b.remoteCommunicationAllowsSteam() {
		t.Errorf("PublicListing 不应允许 Steam 命令")
	}
	cfg.RemoteCommunication = "1"
	if !b.remoteCommunicationAllowsSteam() {
		t.Errorf("数字 1（SteamGroup）应允许 Steam 命令")
	}
	cfg.RemoteCommunication = "0"
	if b.remoteCommunicationAllowsSteam() {
		t.Errorf("数字 0（None）应禁用 Steam 命令")
	}
}

func TestMessagePrefix(t *testing.T) {
	b := newCommandTestBot(t)
	if got := b.messagePrefix(); got != "" {
		t.Errorf("无 GlobalConfig 时前缀应为空，实际 %q", got)
	}
	gc := config.Defaults()
	gc.SteamMessagePrefix = "/me "
	b.GlobalConfig = gc
	if got := b.messagePrefix(); got != "/me " {
		t.Errorf("SteamMessagePrefix 应为 /me ，实际 %q", got)
	}
}

func TestCheckSteamProtocolsWebSocketWarns(t *testing.T) {
	b := newCommandTestBot(t)
	gc := config.Defaults()
	gc.SteamProtocols = "WebSocket"
	b.GlobalConfig = gc
	b.checkSteamProtocols() // 不应 panic
}

func TestCommandHelpWithCommand(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!help status")
	if !strings.Contains(reply, "status") || !strings.Contains(reply, "挂机进度") {
		t.Errorf("!help status 应返回详细用法，实际: %q", reply)
	}
	reply = b.handleCommandText(76561198000000001, "!help nonexistent")
	if !strings.Contains(reply, "未知命令") {
		t.Errorf("!help 未知命令应提示，实际: %q", reply)
	}
}

func TestStripGroupTarget(t *testing.T) {
	prefix := "!"
	botName := "testbot"
	cases := []struct {
		msg      string
		expected string
	}{
		{"!testbot status", "!status"},
		{"!TESTBOT status", "!status"},
		{"!all status", "!status"},
		{"!other status", ""},
		{"!status", ""},
		{"!testbot", ""},
		{"!testbot pause 2h", "!pause 2h"},
	}
	for _, c := range cases {
		got := stripGroupTarget(c.msg, prefix, botName)
		if got != c.expected {
			t.Errorf("stripGroupTarget(%q) = %q，期望 %q", c.msg, got, c.expected)
		}
	}
}

func TestCommandBatchFromFile(t *testing.T) {
	b := newCommandTestBot(t)
	bgrFile := filepath.Join(b.configDir, b.Name+".games-to-redeem-in-background")
	if err := os.WriteFile(bgrFile, []byte("AAAAA-BBBBB-CCCCC\n"), 0o600); err != nil {
		t.Fatalf("写入后台文件失败: %v", err)
	}
	reply := b.handleCommandText(76561198000000001, "!batch")
	if !strings.Contains(reply, "尚未登录") {
		t.Errorf("未登录时 !batch 应提示未登录，实际: %q", reply)
	}
	if !strings.Contains(reply, "后台列表已保留") {
		t.Errorf("失败时后台列表应保留，实际: %q", reply)
	}
	data, _ := os.ReadFile(bgrFile)
	if len(strings.TrimSpace(string(data))) == 0 {
		t.Errorf("兑换失败时后台列表不应清空")
	}
}

func TestCommandBatchEmptyFile(t *testing.T) {
	b := newCommandTestBot(t)
	reply := b.handleCommandText(76561198000000001, "!batch")
	if !strings.Contains(reply, "后台兑换列表为空") {
		t.Errorf("空列表应提示，实际: %q", reply)
	}
}

func TestStatusTextWithProgress(t *testing.T) {
	b := newCommandTestBot(t)
	next := time.Now().Add(5 * time.Minute)
	b.cardsFarmer = &CardsFarmer{
		bot:         b,
		log:         b.Log,
		nextCheckAt: next,
		gamesToFarm: []GameInfo{
			{AppID: 730, Name: "CS2", CardsRemaining: 2, TotalCards: 5},
		},
	}
	text := b.statusText()
	if !strings.Contains(text, "已掉落 3/5，60%") {
		t.Errorf("statusText 应包含挂机进度，实际: %q", text)
	}
	if !strings.Contains(text, "下次检查:") {
		t.Errorf("statusText 应包含下次检查时间，实际: %q", text)
	}
}
