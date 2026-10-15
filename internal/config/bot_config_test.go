package config

import (
	"strings"
	"testing"
)

func TestSetSteamPasswordPlain(t *testing.T) {
	t.Setenv("ASF_CRYPTKEY", "")
	cfg := DefaultBotConfig()
	cfg.SetSteamPassword("secret")
	if cfg.SteamPassword != "secret" {
		t.Errorf("未启用加密时应明文存储，实际: %q", cfg.SteamPassword)
	}
	if cfg.PasswordFormat != "PlainText" {
		t.Errorf("PasswordFormat 应为 PlainText，实际: %q", cfg.PasswordFormat)
	}
	if got := cfg.GetDecryptedSteamPassword(); got != "secret" {
		t.Errorf("GetDecryptedSteamPassword = %q", got)
	}
}

func TestSetSteamPasswordEncrypted(t *testing.T) {
	t.Setenv("ASF_CRYPTKEY", strings.Repeat("ab", 32))
	cfg := DefaultBotConfig()
	cfg.SetSteamPassword("secret")
	if !strings.HasPrefix(cfg.SteamPassword, "encrypted:") {
		t.Fatalf("启用加密时应加密存储，实际: %q", cfg.SteamPassword)
	}
	if cfg.PasswordFormat != "AES" {
		t.Errorf("PasswordFormat 应为 AES，实际: %q", cfg.PasswordFormat)
	}
	if got := cfg.GetDecryptedSteamPassword(); got != "secret" {
		t.Errorf("GetDecryptedSteamPassword = %q，期望 secret", got)
	}
}

func TestGetDecryptedSteamPasswordLegacy(t *testing.T) {
	t.Setenv("ASF_CRYPTKEY", "")
	cfg := DefaultBotConfig()
	cfg.SteamPassword = "legacy-plain"
	cfg.PasswordFormat = "PlainText"
	if got := cfg.GetDecryptedSteamPassword(); got != "legacy-plain" {
		t.Errorf("兼容明文配置应原样返回，实际: %q", got)
	}
}
