package ipc

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/asf-go/asf/internal/core"
)

// setupTestASFWithBot 启动一个带 testbot 配置的 ASF 实例，用于契约测试。
func setupTestASFWithBot(t *testing.T) (*core.ASF, *Server) {
	t.Helper()
	dir := t.TempDir()
	cfg := `{
		"AutoRestart": true,
		"CommandPrefix": "!",
		"SteamOwnerID": 76561198000000000,
		"IPC": true,
		"IPCPassword": "testpass"
	}`
	if err := os.WriteFile(filepath.Join(dir, "ASF.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	botCfg := `{
		"Enabled": true,
		"SteamLogin": "testlogin",
		"SteamPassword": "testpass",
		"SteamUserPermissions": {"76561198000000000": "Master"},
		"TradingAllowableSteamIDs": [],
		"TradingBlacklist": []
	}`
	if err := os.WriteFile(filepath.Join(dir, "testbot.json"), []byte(botCfg), 0o600); err != nil {
		t.Fatal(err)
	}

	asf, err := core.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := asf.Start(); err != nil {
		t.Fatal(err)
	}
	srv := New(asf, asf.Log, "testpass")
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	addr := srv.listener.Addr().String()
	t.Setenv("TEST_ADDR_BOT", addr)

	t.Cleanup(func() {
		_ = srv.Stop()
		_ = asf.Stop()
	})
	return asf, srv
}

// getResult 请求指定路径并返回响应的 Result 对象（断言 200 与 Success）。
func getResult(t *testing.T, addr, path string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://"+addr+path, nil)
	req.SetBasicAuth("ipc", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求 %s 失败: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s 状态码应为 200，得到 %d", path, resp.StatusCode)
	}
	var body struct {
		Success bool           `json:"Success"`
		Result  map[string]any `json:"Result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("解析 %s 响应失败: %v", path, err)
	}
	if !body.Success {
		t.Fatalf("%s 响应 Success=false", path)
	}
	return body.Result
}

// assertContainsKeys 断言 result 包含所有期望键（契约：UI 依赖的字段必须存在，允许额外字段）。
func assertContainsKeys(t *testing.T, path string, result map[string]any, keys []string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := result[k]; !ok {
			t.Errorf("契约违约：%s 的 Result 缺少字段 %s", path, k)
		}
	}
}

// TestAPIContractASF 验证 /api/asf 的 Result 包含原版 ASFResponse 的全部字段。
func TestAPIContractASF(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")
	result := getResult(t, addr, "/api/asf")
	assertContainsKeys(t, "/api/asf", result, []string{
		"BuildVariant", "CanUpdate", "GlobalConfig", "MemoryUsage",
		"ProcessStartTime", "Service", "Version",
	})
}

// TestAPIContractBot 验证 /api/bot/ASF 中每个 Bot 对象包含原版 Bot 序列化字段。
func TestAPIContractBot(t *testing.T) {
	setupTestASFWithBot(t)
	addr := os.Getenv("TEST_ADDR_BOT")
	result := getResult(t, addr, "/api/bot/ASF")

	if len(result) == 0 {
		t.Fatal("/api/bot/ASF 的 Result 不应为空（应有 testbot）")
	}
	for botName, raw := range result {
		botObj, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("Bot %s 不是对象", botName)
			continue
		}
		assertContainsKeys(t, "/api/bot/ASF#"+botName, botObj, []string{
			"BotName", "IsConnectedAndLoggedOn", "KeepRunning", "CardsFarmer",
			"s_SteamID", "HasMobileAuthenticator", "BotConfig",
		})
	}
}

// TestAPIContractType 验证 /api/type/{type} 的 Result 包含 Body 与 Properties 契约。
func TestAPIContractType(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")
	result := getResult(t, addr, "/api/type/ArchiSteamFarm.Steam.Storage.BotConfig")
	assertContainsKeys(t, "/api/type/BotConfig", result, []string{"Body", "Properties"})
	props, ok := result["Properties"].(map[string]any)
	if !ok {
		t.Fatal("Properties 应为对象")
	}
	assertContainsKeys(t, "/api/type/BotConfig#Properties", props, []string{"BaseType"})
}

// TestAPIContractStructure 验证 /api/structure/{type} 返回默认配置对象。
func TestAPIContractStructure(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")
	result := getResult(t, addr, "/api/structure/ArchiSteamFarm.Steam.Storage.BotConfig")
	if len(result) == 0 {
		t.Error("/api/structure/BotConfig 的 Result 不应为空")
	}
}