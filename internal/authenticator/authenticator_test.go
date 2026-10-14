package authenticator

import (
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"
)

func testSecret(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func TestGenerateCodeFormat(t *testing.T) {
	a, err := Import(testSecret(t), testSecret(t), "android:test")
	if err != nil {
		t.Fatal(err)
	}
	code, err := a.GenerateCodeAt(time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 5 {
		t.Errorf("Steam 验证码应为 5 位，得到 %q", code)
	}
	for _, ch := range code {
		isAlphaNum := (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
		if !isAlphaNum {
			t.Errorf("验证码含非法字符 %q", ch)
		}
	}
}

func TestGenerateCodeDeterministic(t *testing.T) {
	a, _ := Import(testSecret(t), testSecret(t), "android:test")
	at := time.Unix(1700000000, 0)
	c1, _ := a.GenerateCodeAt(at)
	c2, _ := a.GenerateCodeAt(at)
	if c1 != c2 {
		t.Errorf("同一时间生成的验证码应一致: %q != %q", c1, c2)
	}
}

func TestGenerateConfirmationCode(t *testing.T) {
	a, _ := Import(testSecret(t), testSecret(t), "android:test")
	code, err := a.GenerateConfirmationCode("conf")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 8 {
		t.Errorf("确认码应为 8 位，得到 %q", code)
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ma.json")
	a, _ := Import(testSecret(t), testSecret(t), "android:test")
	if err := a.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SharedSecret != a.SharedSecret {
		t.Error("SharedSecret 不一致")
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexist.json"))
	if err == nil {
		t.Error("加载不存在的文件应报错")
	}
}
