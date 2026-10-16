package ipc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asf-go/asf/internal/core"
)

func setupTestASF(t *testing.T) (*core.ASF, *Server) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ASF.json")
	cfg := `{
		"AutoRestart": true,
		"CommandPrefix": "!",
		"SteamOwnerID": 76561198000000000,
		"IPC": true,
		"IPCPassword": "testpass"
	}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
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

	t.Cleanup(func() {
		_ = srv.Stop()
		_ = asf.Stop()
	})

	// 供请求使用
	t.Setenv("TEST_ADDR", addr)
	return asf, srv
}

func TestASFEndpoint(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")

	req, _ := http.NewRequest("GET", "http://"+addr+"/api/ASF", nil)
	req.SetBasicAuth("ipc", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码应为 200，得到 %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["Success"] != true {
		t.Errorf("success 应为 true")
	}
	result, ok := body["Result"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 result")
	}
	if result["Version"] == "" || result["Version"] == nil {
		t.Errorf("缺少 version")
	}
	if _, ok := result["Bots"]; !ok {
		t.Errorf("缺少 bots")
	}
}

func TestUnauthorized(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")

	resp, err := http.Get("http://" + addr + "/api/ASF")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无认证应返回 401，得到 %d", resp.StatusCode)
	}
}

func TestCommandsEndpoint(t *testing.T) {
	setupTestASF(t)
	addr := os.Getenv("TEST_ADDR")

	body := []byte(`{"command":"status"}`)
	req, _ := http.NewRequest("POST", "http://"+addr+"/api/commands", bytes.NewReader(body))
	req.SetBasicAuth("ipc", "testpass")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码应为 200，得到 %d", resp.StatusCode)
	}
}
func TestIPCBanOnRepeatedFailures(t *testing.T) {
	setupTestASF(t)
	clearFailedAuthorizations()
	addr := os.Getenv("TEST_ADDR")

	client := &http.Client{}
	for i := 0; i < maxFailedAuthorizationAttempts; i++ {
		req, _ := http.NewRequest("GET", "http://"+addr+"/api/ASF", nil)
		req.SetBasicAuth("ipc", "wrongpass")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("第 %d 次错误密码应返回 401，得到 %d", i+1, resp.StatusCode)
		}
	}

	// 达到阈值后应返回 403
	req, _ := http.NewRequest("GET", "http://"+addr+"/api/ASF", nil)
	req.SetBasicAuth("ipc", "wrongpass")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("封禁后应返回 403，得到 %d", resp.StatusCode)
	}

	// 封禁后即使密码正确也无法访问任何端点（原版行为）
	req, _ = http.NewRequest("GET", "http://"+addr+"/api/IPC/Bans", nil)
	req.SetBasicAuth("ipc", "testpass")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("封禁后访问 Bans 应返回 403，得到 %d", resp.StatusCode)
	}

	// 封禁列表应包含本机地址
	banned := getCurrentlyBannedIPs()
	found := false
	for _, ip := range banned {
		if ip == "127.0.0.1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("封禁列表应包含 127.0.0.1，得到 %#v", banned)
	}

	// 单元级解封后恢复 401
	if !unbanIP("127.0.0.1") {
		t.Fatal("unbanIP 应返回 true")
	}
	req, _ = http.NewRequest("GET", "http://"+addr+"/api/ASF", nil)
	req.SetBasicAuth("ipc", "wrongpass")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("解封后错误密码应返回 401，得到 %d", resp.StatusCode)
	}
}

func TestIPCBansEndpoint(t *testing.T) {
	setupTestASF(t)
	clearFailedAuthorizations()
	addr := os.Getenv("TEST_ADDR")

	// 无认证访问 Bans 列表应被拒绝（401）
	resp, err := http.Get("http://" + addr + "/api/IPC/Bans")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无认证访问 Bans 应返回 401，得到 %d", resp.StatusCode)
	}

	// 正确认证返回成功
	req, _ := http.NewRequest("GET", "http://"+addr+"/api/IPC/Bans", nil)
	req.SetBasicAuth("ipc", "testpass")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("认证后应返回 200，得到 %d", resp.StatusCode)
	}
}
