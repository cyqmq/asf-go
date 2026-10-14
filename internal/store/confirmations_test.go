package store

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/asf-go/asf/internal/steam"
)

// roundTripFunc 自定义 RoundTripper，便于将请求重定向到测试服务器。
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// newTestWS 构造 Web 会话，其 HTTP 客户端将所有请求转发到测试服务器。
func newTestWS(t *testing.T, server *httptest.Server) *steam.WebSession {
	t.Helper()
	target, _ := url.Parse(server.URL)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req2 := req.Clone(req.Context())
			req2.URL.Scheme = target.Scheme
			req2.URL.Host = target.Host
			return http.DefaultTransport.RoundTrip(req2)
		}),
	}
	ws, err := steam.NewWebSessionWithClient("sid", "login", client)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestGetConfirmations(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"conf": []map[string]any{
				{"id": "123", "nonce": "456", "type": 2, "creator_id": "76561198000000000"},
			},
		})
	}))
	defer server.Close()

	ws := newTestWS(t, server)
	confs, err := GetConfirmations(ws, 76561198000000000, "android:test", "hash", 1700000000)
	if err != nil {
		t.Fatalf("GetConfirmations 失败: %v", err)
	}
	if len(confs) != 1 {
		t.Fatalf("应返回 1 条确认，得到 %d", len(confs))
	}
	if uint64(confs[0].ID) != 123 || uint64(confs[0].Nonce) != 456 {
		t.Fatalf("确认字段解析错误: %+v", confs[0])
	}
	if !strings.Contains(gotQuery, "a=76561198000000000") || !strings.Contains(gotQuery, "tag=conf") {
		t.Fatalf("查询参数不正确: %s", gotQuery)
	}
}

func TestHandleConfirmations(t *testing.T) {
	var gotForm string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm.Encode()
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	ws := newTestWS(t, server)
	confs := []Confirmation{{ID: 123, Nonce: 456}}
	ok, err := HandleConfirmations(ws, 76561198000000000, "android:test", "hash", 1700000000, confs, true)
	if err != nil {
		t.Fatalf("HandleConfirmations 失败: %v", err)
	}
	if !ok {
		t.Fatal("应返回 success=true")
	}
	for _, want := range []string{"op=allow", "cid%5B%5D=123", "ck%5B%5D=456", "tag=conf"} {
		if !strings.Contains(gotForm, want) {
			t.Fatalf("表单缺少 %s: %s", want, gotForm)
		}
	}
}

func TestHandleConfirmationsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
	}))
	defer server.Close()

	ws := newTestWS(t, server)
	if _, err := HandleConfirmations(ws, 1, "android:test", "hash", 1700000000, nil, true); err == nil {
		t.Fatal("空确认列表应返回错误")
	}
}

func TestConfirmationsNilSession(t *testing.T) {
	if _, err := GetConfirmations(nil, 1, "d", "h", 1); err == nil {
		t.Fatal("nil Web 会话应返回错误")
	}
}
