// Package authenticator 移植自 ASF 的 MobileAuthenticator 插件，负责 Steam Guard 2FA。
package authenticator

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cyqmq/go-steam/v3/totp"
)

// Authenticator 表示一个 Steam 移动令牌。
type Authenticator struct {
	SharedSecret   string `json:"shared_secret"`
	IdentitySecret string `json:"identity_secret"`
	SerialNumber   string `json:"serial_number"`
	DeviceID       string `json:"device_id"`
}

// Import 从 shared_secret 与 identity_secret（Base64）导入令牌。
func Import(sharedSecret, identitySecret string) (*Authenticator, error) {
	if _, err := base64.StdEncoding.DecodeString(sharedSecret); err != nil {
		return nil, fmt.Errorf("shared_secret 不是有效 Base64: %w", err)
	}
	return &Authenticator{SharedSecret: sharedSecret, IdentitySecret: identitySecret}, nil
}

// GenerateCode 生成当前 Steam Guard 5 位字母数字验证码。
func (a *Authenticator) GenerateCode() (string, error) {
	t := totp.NewTimedTotp(a.SharedSecret, time.Now())
	return t.GenerateCode()
}

// GenerateCodeAt 生成指定时间的验证码（用于测试）。
func (a *Authenticator) GenerateCodeAt(at time.Time) (string, error) {
	t := totp.NewTimedTotp(a.SharedSecret, at)
	return t.GenerateCode()
}

// GenerateConfirmationCode 基于 identity_secret 生成 8 位确认码。
// tag 通常为 "conf" 或 "allow"。
func (a *Authenticator) GenerateConfirmationCode(tag string) (string, error) {
	return generateConfirmationCode(a.IdentitySecret, time.Now().Unix(), tag)
}

// generateConfirmationCode 实现 Steam 确认码算法。
func generateConfirmationCode(identitySecret string, timestamp int64, tag string) (string, error) {
	secret, err := base64.StdEncoding.DecodeString(identitySecret)
	if err != nil {
		return "", fmt.Errorf("identity_secret 不是有效 Base64: %w", err)
	}
	message := []byte(fmt.Sprintf("%d%s", timestamp, tag))
	mac := hmac.New(sha1.New, secret)
	mac.Write(message)
	hash := mac.Sum(nil)
	offset := hash[19] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%08d", code%100000000), nil
}

// Save 将令牌保存到文件。
func (a *Authenticator) Save(path string) error {
	data, err := json.MarshalIndent(a, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load 从文件加载令牌。
func Load(path string) (*Authenticator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a Authenticator
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	if a.SharedSecret == "" {
		return nil, errors.New("缺少 shared_secret")
	}
	return &a, nil
}

// NormalizeCode 去除验证码中的空格/小写。
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
