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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码应为 200，得到 %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["success"] != true {
		t.Errorf("success 应为 true")
	}
	result, ok := body["result"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 result")
	}
	if result["version"] == "" || result["version"] == nil {
		t.Errorf("缺少 version")
	}
	if _, ok := result["bots"]; !ok {
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码应为 200，得到 %d", resp.StatusCode)
	}
}