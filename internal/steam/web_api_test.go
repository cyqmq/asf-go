package steam

import (
	"net/http"
	"testing"
)

func TestNewWebSessionWithProxy(t *testing.T) {
	ws, err := newWebSession("sid", "secure", "http://127.0.0.1:8080", "user", "pass")
	if err != nil {
		t.Fatalf("创建 Web 会话失败: %v", err)
	}
	client := ws.HTTPClient()
	if client == nil {
		t.Fatal("HTTP 客户端为空")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型应为 *http.Transport，实际 %T", client.Transport)
	}
	if transport.Proxy == nil {
		t.Errorf("代理 Transport 未设置 Proxy 函数")
	}
}

func TestNewWebSessionWithoutProxy(t *testing.T) {
	ws, err := newWebSession("sid", "secure", "", "", "")
	if err != nil {
		t.Fatalf("创建 Web 会话失败: %v", err)
	}
	if ws.HTTPClient().Transport != nil {
		t.Errorf("无代理时 Transport 应为 nil")
	}
}
