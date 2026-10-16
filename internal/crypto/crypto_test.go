package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecryptAES(t *testing.T) {
	plain := "Hello ArchiSteamFarm 1234567890"
	encrypted, err := EncryptAES(plain)
	if err != nil {
		t.Fatalf("EncryptAES 失败: %v", err)
	}
	decrypted, err := DecryptAES(encrypted)
	if err != nil {
		t.Fatalf("DecryptAES 失败: %v", err)
	}
	if decrypted != plain {
		t.Errorf("DecryptAES(EncryptAES(%q)) = %q", plain, decrypted)
	}
}

func TestEncryptAESRandomIV(t *testing.T) {
	plain := "same input"
	a, err := EncryptAES(plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptAES(plain)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("两次加密不应产生相同密文（随机 IV）")
	}
}

func TestDecryptAESInvalid(t *testing.T) {
	if _, err := DecryptAES("not-base64!!!"); err == nil {
		t.Error("无效 Base64 应返回错误")
	}
	if _, err := DecryptAES(""); err == nil {
		t.Error("空密文应返回错误")
	}
}

func TestEncryptLoginKeyDisabledByDefault(t *testing.T) {
	t.Setenv(EnvironmentVariableCryptKey, "")
	value := EncryptLoginKey("my-login-key")
	if value != "my-login-key" {
		t.Errorf("未启用加密时 EncryptLoginKey 应返回原文，实际: %q", value)
	}
}

func TestDecryptLoginKeyPlain(t *testing.T) {
	t.Setenv(EnvironmentVariableCryptKey, "")
	if got := DecryptLoginKey("plain-key"); got != "plain-key" {
		t.Errorf("非加密值应原样返回，实际: %q", got)
	}
}

func TestEncryptLoginKeyWithCryptKey(t *testing.T) {
	// 32 字节 hex 密钥
	key := strings.Repeat("ab", 32)
	t.Setenv(EnvironmentVariableCryptKey, key)

	encrypted := EncryptLoginKey("secret-login-key")
	if encrypted == "secret-login-key" {
		t.Fatal("启用 ASF_CRYPTKEY 后应加密登录 Key")
	}
	if !strings.HasPrefix(encrypted, "encrypted:") {
		t.Fatalf("加密值应带 encrypted: 前缀，实际: %q", encrypted)
	}
	decrypted := DecryptLoginKey(encrypted)
	if decrypted != "secret-login-key" {
		t.Errorf("解密登录 Key = %q，期望 secret-login-key", decrypted)
	}
}
