package bot

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/asf-go/asf/internal/inventory"
	"github.com/asf-go/asf/internal/steam"
	"github.com/asf-go/asf/internal/store"
	"github.com/asf-go/asf/internal/trading"
	"github.com/asf-go/asf/internal/web"
)

func (b *Bot) Loot(targetSteamID uint64) (uint64, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return 0, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws, err := b.steamClient.WebSession()
	if err != nil {
		return 0, fmt.Errorf("Web 会话不可用: %w", err)
	}
	steamID := b.steamClient.SteamID()
	if steamID == 0 {
		return 0, fmt.Errorf("Bot %s SteamID 无效", b.Name)
	}

	inv, err := inventory.GetOwnInventory(ws, steamID, 753, 6)
	if err != nil {
		return 0, fmt.Errorf("获取库存失败: %w", err)
	}

	var items []trading.TradeItem
	for _, item := range inv.Items {
		if !isLootable(item) {
			continue
		}
		items = append(items, trading.TradeItem{
			AppID:     753,
			ContextID: 6,
			AssetID:   item.ID,
			Amount:    item.Amount,
		})
	}
	if len(items) == 0 {
		return 0, fmt.Errorf("库存中没有可转移物品")
	}

	offerID, err := trading.SendTradeOffer(ws, b.SteamAPIKey, targetSteamID, items, fmt.Sprintf("Loot from %s", b.Name))
	if err != nil {
		return 0, fmt.Errorf("发送交易报价失败: %w", err)
	}
	b.Log.Info("已发送报价 %d 给 %d，共 %d 个物品", offerID, targetSteamID, len(items))
	return offerID, nil
}

// AddLicense 激活免费许可。支持 sub/<subid>、app/<appid> 或商店链接。

func (b *Bot) AddLicense(identifier string) (string, error) {
	idType, id, ok := ParseGameIdentifier(identifier, GameIdentifierPackage)
	if !ok {
		return "", fmt.Errorf("无效游戏标识符: %s", identifier)
	}
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}

	switch idType {
	case GameIdentifierPackage:
		ws, err := b.steamClient.WebSession()
		if err != nil {
			return "", fmt.Errorf("Web 会话不可用: %w", err)
		}
		result, err := store.AddFreeLicense(ws, uint64(id))
		if err != nil {
			return "", fmt.Errorf("激活失败: %w", err)
		}
		b.Log.Info("已请求激活免费许可 sub/%d: %s", id, result)
		return fmt.Sprintf("sub/%d: %s", id, result), nil
	case GameIdentifierApplication:
		if b.steamClient == nil || !b.steamClient.LoggedOn() {
			return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
		}
		result, err := b.steamClient.RequestFreeLicense(uint32(id))
		if err != nil {
			return "", fmt.Errorf("app/%d 激活失败: %w", id, err)
		}
		b.Log.Info("已请求激活免费许可 app/%d: result=%d, granted_apps=%v, granted_packages=%v", id, result.Result, result.GrantedApps, result.GrantedPackages)
		return fmt.Sprintf("app/%d: 结果 %d，授权应用 %v，授权包 %v", id, result.Result, result.GrantedApps, result.GrantedPackages), nil
	default:
		return "", fmt.Errorf("不支持的游戏标识符类型")
	}
}

// CheckAppBeta 提交指定 appID 的 beta 密码，返回可用 beta 分支及其加密密钥。

func (b *Bot) CheckAppBeta(appID uint32, password string) (string, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	result, err := b.steamClient.CheckAppBetaPassword(appID, password)
	if err != nil {
		return "", fmt.Errorf("beta 检查失败: %w", err)
	}
	var lines []string
	for _, bp := range result.BetaPasswords {
		lines = append(lines, fmt.Sprintf("%s: %x", bp.Name, bp.Password))
	}
	if len(lines) == 0 {
		return fmt.Sprintf("app/%d: 无可用 beta 分支（结果 %d）", appID, result.Result), nil
	}
	return fmt.Sprintf("app/%d: %s", appID, strings.Join(lines, ", ")), nil
}

// Owns 查询本 Bot 是否拥有指定游戏。identifiers 支持 app/<appid> 或裸数字。

func (b *Bot) Owns(identifiers []string) (string, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.SteamAPIKey == "" {
		return "", fmt.Errorf("未配置 SteamWebApiKey，无法查询游戏库")
	}
	steamID := fmt.Sprintf("%d", b.steamClient.SteamID())
	games, err := steam.FetchOwnedGames(b.SteamAPIKey, steamID)
	if err != nil {
		return "", fmt.Errorf("查询游戏库失败: %w", err)
	}

	var lines []string
	for _, ident := range identifiers {
		idType, id, ok := ParseGameIdentifier(ident, GameIdentifierApplication)
		if !ok || idType != GameIdentifierApplication {
			lines = append(lines, fmt.Sprintf("%s: 无效 appid", ident))
			continue
		}
		if name, ok := games[id]; ok {
			lines = append(lines, fmt.Sprintf("app/%d: 已拥有 (%s)", id, name))
		} else {
			lines = append(lines, fmt.Sprintf("app/%d: 未拥有", id))
		}
	}
	b.Log.Info("Bot %s 查询了 %d 个游戏所有权", b.Name, len(identifiers))
	return strings.Join(lines, "; "), nil
}

// Redeem 兑换一个或多个 Steam 激活码，返回每个激活码的结果。

func (b *Bot) Redeem(keys []string) (string, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return "", fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws, err := b.steamClient.WebSession()
	if err != nil {
		return "", fmt.Errorf("Web 会话不可用: %w", err)
	}

	var lines []string
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			lines = append(lines, "空激活码，跳过")
			continue
		}
		result, err := store.RedeemKey(ws, key)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: 兑换失败: %v", key, err))
			continue
		}
		b.Log.Info("Bot %s 兑换激活码 %s: %s", b.Name, key, result)
		lines = append(lines, fmt.Sprintf("%s: %s", key, result))
	}
	return strings.Join(lines, "; "), nil
}

// PointsBalance 查询本 Bot 的 Steam 积分余额（需要 SteamWebApiKey）。

func (b *Bot) PointsBalance() (int64, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return 0, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	if b.SteamAPIKey == "" {
		return 0, fmt.Errorf("未配置 SteamWebApiKey")
	}
	steamID := fmt.Sprintf("%d", b.steamClient.SteamID())
	return steam.FetchPointsBalance(b.SteamAPIKey, steamID)
}

// Update 检查 asf-go 自身是否有新版本；若有则下载对应平台二进制、替换并重启。

func (b *Bot) Update() (string, error) {
	client := web.NewGitHubClient()
	release, err := client.GetLatestRelease("cyqmq", "asf-go", true)
	if err != nil {
		// asf-go 尚无 Release 时回退到上游 ASF 版本信息
		upstream, upErr := client.GetLatestRelease("JustArchiNET", "ArchiSteamFarm", true)
		if upErr != nil {
			return "", fmt.Errorf("检查更新失败: %w", err)
		}
		return fmt.Sprintf("asf-go 暂无 Release，上游 ASF 最新版本: %s", upstream.TagName), nil
	}

	if web.CompareVersions(release.TagName, b.ASFVersion) <= 0 {
		return fmt.Sprintf("当前已是最新版本 %s", b.ASFVersion), nil
	}

	assetName := fmt.Sprintf("asf-go-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	var asset *web.GitHubAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		return "", fmt.Errorf("新版本 %s 中未找到当前平台更新包 %s", release.TagName, assetName)
	}

	b.Log.Info("发现新版本 %s，正在下载 %s ...", release.TagName, asset.Name)
	tmpFile, err := os.CreateTemp("", "asf-go-update-*")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	if err := client.DownloadAsset(asset, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("下载更新包失败: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("设置执行权限失败: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("定位当前可执行文件失败: %w", err)
	}

	// 备份旧文件再替换，避免替换失败导致程序不可用
	backupPath := exe + ".old"
	_ = os.Remove(backupPath)
	if err := os.Rename(exe, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("备份旧可执行文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Rename(backupPath, exe)
		return "", fmt.Errorf("替换可执行文件失败: %w", err)
	}

	b.Log.Info("更新完成，正在重启 ...")
	cmd := exec.Command(exe)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("重启失败: %w", err)
	}

	if b.OnExit != nil {
		b.OnExit()
	}
	return fmt.Sprintf("已更新到 %s，正在重启", release.TagName), nil
}

// UpdateCheck 检查上游 ASF 是否有新版本。
// UnpackBoosters 打开库存中所有补充包，返回开包数量。

// SetPrivacy 修改 Steam 社区隐私设置（对应原版 !privacy）。
func (b *Bot) SetPrivacy(settings store.PrivacySettings, commentPermission int) (bool, error) {
	if b.steamClient == nil || !b.steamClient.LoggedOn() {
		return false, fmt.Errorf("Bot %s 尚未登录", b.Name)
	}
	ws := b.steamClient.WebSessionOrNil()
	if ws == nil {
		return false, fmt.Errorf("Bot %s 无 Web 会话", b.Name)
	}
	return store.SetPrivacy(ws, b.SteamID(), settings, commentPermission)
}
