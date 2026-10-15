package ipc

import "testing"

func TestCorsAllowOriginDefault(t *testing.T) {
	if got := corsAllowOrigin(nil, "https://example.com"); got != "*" {
		t.Errorf("空白名单应返回 *，实际: %q", got)
	}
}

func TestCorsAllowOriginWhitelist(t *testing.T) {
	allowed := []string{"https://asf.example.com"}
	if got := corsAllowOrigin(allowed, "https://asf.example.com"); got != "https://asf.example.com" {
		t.Errorf("白名单来源应回显，实际: %q", got)
	}
	if got := corsAllowOrigin(allowed, "https://evil.example.com"); got != "" {
		t.Errorf("非白名单来源应返回空，实际: %q", got)
	}
}
