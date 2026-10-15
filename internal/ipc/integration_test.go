package ipc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// TestASFUIKeyEndpoints 验证 ASF-ui 依赖的关键 API 端点均可访问（集成测试）。
func TestASFUIKeyEndpoints(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")
	client := &http.Client{}

	paths := []string{
		"/api/asf",
		"/api/bot/ASF",
		"/api/bots",
		"/api/type/ArchiSteamFarm.Steam.Storage.BotConfig",
		"/api/structure/ArchiSteamFarm.Steam.Storage.BotConfig",
		"/api/plugins",
	}
	for _, path := range paths {
		req, _ := http.NewRequest("GET", "http://"+addr+path, nil)
		req.SetBasicAuth("ipc", "testpass")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("请求 %s 失败: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s 状态码应为 200，得到 %d", path, resp.StatusCode)
		}
	}
}

// TestCommandsEndpointViaIPC 验证 POST /api/command 可解析并执行（status 命令）。
func TestCommandsEndpointViaIPC(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")

	body, _ := json.Marshal(map[string]string{"command": "status"})
	req, _ := http.NewRequest("POST", "http://"+addr+"/api/command", bytes.NewReader(body))
	req.SetBasicAuth("ipc", "testpass")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码应为 200，得到 %d", resp.StatusCode)
	}
}
