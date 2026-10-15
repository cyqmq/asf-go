package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/asf-go/asf/internal/authenticator"
	"github.com/asf-go/asf/internal/store"
)

func (b *Bot) startConfirmations() {
	if b.auth == nil {
		return
	}
	b.confirmationsStopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				b.processConfirmations()
			case <-b.confirmationsStopCh:
				return
			}
		}
	}()
}

// processConfirmations 查询并自动确认所有待确认项（交易/市场挂单）。

func (b *Bot) processConfirmations() {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return
	}
	ws := b.steamClient.WebSessionOrNil()
	if ws == nil {
		return
	}
	code, err := b.auth.GenerateConfirmationCode("conf")
	if err != nil {
		b.Log.Warn("生成确认码失败: %v", err)
		return
	}
	deviceID := b.auth.DeviceID
	if deviceID == "" {
		deviceID = "android:asf-go"
	}
	now := time.Now().Unix()

	confirmations, err := store.GetConfirmations(ws, b.SteamID(), deviceID, code, now)
	if err != nil {
		b.Log.Debug("查询确认列表失败: %v", err)
		return
	}
	if len(confirmations) == 0 {
		return
	}

	b.Log.Info("发现 %d 个待确认项，自动确认", len(confirmations))
	ok, err := store.HandleConfirmations(ws, b.SteamID(), deviceID, code, now, confirmations, true)
	if err != nil {
		b.Log.Warn("自动确认失败: %v", err)
		return
	}
	if ok {
		b.Log.Info("已确认 %d 个待确认项", len(confirmations))
	} else {
		b.Log.Warn("Steam 返回确认失败")
	}
}

// GetConfirmations 查询当前待确认列表（需要已登录且配置了移动验证器）。

func (b *Bot) GetConfirmations() ([]store.Confirmation, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return nil, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.auth == nil {
		return nil, fmt.Errorf("Bot %s 未配置移动验证器", b.Name)
	}
	ws := b.steamClient.WebSessionOrNil()
	if ws == nil {
		return nil, fmt.Errorf("Bot %s 无 Web 会话", b.Name)
	}
	code, err := b.auth.GenerateConfirmationCode("conf")
	if err != nil {
		return nil, fmt.Errorf("生成确认码失败: %w", err)
	}
	deviceID := b.auth.DeviceID
	if deviceID == "" {
		deviceID = "android:asf-go"
	}
	confirmations, err := store.GetConfirmations(ws, b.SteamID(), deviceID, code, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return confirmations, nil
}

// HandleConfirmations 接受或拒绝指定的确认项。

func (b *Bot) HandleConfirmations(accept bool, confirmations []store.Confirmation) (bool, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return false, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.auth == nil {
		return false, fmt.Errorf("Bot %s 未配置移动验证器", b.Name)
	}
	ws := b.steamClient.WebSessionOrNil()
	if ws == nil {
		return false, fmt.Errorf("Bot %s 无 Web 会话", b.Name)
	}
	code, err := b.auth.GenerateConfirmationCode("conf")
	if err != nil {
		return false, fmt.Errorf("生成确认码失败: %w", err)
	}
	deviceID := b.auth.DeviceID
	if deviceID == "" {
		deviceID = "android:asf-go"
	}
	return store.HandleConfirmations(ws, b.SteamID(), deviceID, code, time.Now().Unix(), confirmations, accept)
}

// startTrading 启动交易报价轮询（立即执行一次，之后按 TradingPeriod 小时轮询）。

func (b *Bot) loginKeyPath() string {
	return filepath.Join(b.configDir, fmt.Sprintf("%s.loginkey.json", b.Name))
}

// maFilePath 返回移动令牌文件路径。

func (b *Bot) maFilePath() string {
	return filepath.Join(b.configDir, fmt.Sprintf("%s.maFile.json", b.Name))
}

func (b *Bot) loadAuthenticator() *authenticator.Authenticator {
	a, err := authenticator.Load(b.maFilePath())
	if err != nil {
		return nil
	}
	return a
}

// HasAuthenticator 返回是否已配置移动令牌。

func (b *Bot) HasAuthenticator() bool {
	return b.auth != nil
}

// Auth 返回移动令牌实例（未配置时为 nil）。

func (b *Bot) Auth() *authenticator.Authenticator {
	return b.auth
}

// TwoFactorCode 返回当前 Steam Guard 验证码。

func (b *Bot) TwoFactorCode() (string, error) {
	if b.auth == nil {
		return "", fmt.Errorf("Bot %s 未配置移动令牌", b.Name)
	}
	return b.auth.GenerateCode()
}

// ImportAuthenticator 导入并保存移动令牌（shared_secret / identity_secret / 可选 device_id）。

func (b *Bot) ImportAuthenticator(sharedSecret, identitySecret, deviceID string) error {
	a, err := authenticator.Import(sharedSecret, identitySecret, deviceID)
	if err != nil {
		return err
	}
	if err := a.Save(b.maFilePath()); err != nil {
		return err
	}
	b.auth = a
	b.Log.Info("移动令牌已导入并保存")
	return nil
}

type loginKeyFile struct {
	LoginKey string `json:"LoginKey"`
}

func (b *Bot) loadLoginKey() string {
	data, err := os.ReadFile(b.loginKeyPath())
	if err != nil {
		return ""
	}
	var f loginKeyFile
	if err := json.Unmarshal(data, &f); err != nil {
		return ""
	}
	return f.LoginKey
}

func (b *Bot) saveLoginKey() {
	f := loginKeyFile{LoginKey: b.loginKey}
	data, err := json.MarshalIndent(&f, "", "\t")
	if err != nil {
		return
	}
	_ = os.WriteFile(b.loginKeyPath(), data, 0o600)
}
