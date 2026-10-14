package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.9.9", "2.0.0", -1},
		{"v2.1.0", "2.0.9", 1},
		{"1.0", "1.0.0", 0},
		{"2", "1.9", 1},
	}
	for _, c := range cases {
		got := CompareVersions(c.a, c.b)
		if got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, 期望 %d", c.a, c.b, got, c.want)
		}
	}
}
func TestDownloadAsset(t *testing.T) {
	content := []byte("fake-binary-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer server.Close()

	client := NewGitHubClient()
	dest := filepath.Join(t.TempDir(), "asf-go-update")
	asset := &GitHubAsset{Name: "asf-go-linux-amd64", URL: server.URL}

	if err := client.DownloadAsset(asset, dest); err != nil {
		t.Fatalf("DownloadAsset 失败: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("下载内容不匹配，得到 %q", got)
	}
}

func TestDownloadAssetInvalid(t *testing.T) {
	client := NewGitHubClient()
	if err := client.DownloadAsset(nil, "/tmp/x"); err == nil {
		t.Fatal("nil asset 应返回错误")
	}
}
