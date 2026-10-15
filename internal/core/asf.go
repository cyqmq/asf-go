// Package core 移植自 ArchiSteamFarm.Core.ASF（C# 主入口的精简版）。
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/asf-go/asf/internal/bot"
	"github.com/asf-go/asf/internal/config"
	"github.com/asf-go/asf/internal/logging"
	"github.com/asf-go/asf/internal/plugins"
	"github.com/asf-go/asf/internal/web"
)

// Version 对应 ASF 版本信息（后续可自动获取）。
const Version = "0.1.0-go"

// ASF 核心对象，负责配置加载、日志与生命周期。
type ASF struct {
	ConfigPath string
	Config     *config.GlobalConfig
	Log        *logging.Logger
	Plugins    *plugins.Manager
	OnExit     func()

	startTime time.Time
	bots      map[string]*bot.Bot
}

// New 创建 ASF 核心实例并加载配置。
// configPath 为 ASF 配置目录（应包含 ASF.json 与每个 Bot 的 <BotName>.json）。
func New(configPath string) (*ASF, error) {
	log, err := logging.New("ASF", filepath.Join(configPath, "asf-go.log"), logging.Info)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(filepath.Join(configPath, "ASF.json"))
	if err != nil {
		log.Close()
		return nil, err
	}

	asf := &ASF{
		ConfigPath: configPath,
		Config:     cfg,
		Log:        log,
		Plugins:    plugins.NewManager(),
		startTime:  time.Now(),
		bots:       map[string]*bot.Bot{},
	}
	asf.registerBuiltinPlugins()
	return asf, nil
}

// Start 初始化并启动所有启用的 Bot。
func (a *ASF) Start() error {
	a.Log.Info("ASF Go 版 %s 启动", Version)
	a.Log.Info("配置目录: %s", a.ConfigPath)
	a.Log.Info("默认 Bot: %s", a.Config.DefaultBot)
	a.Log.Info("SteamOwnerID: %d", a.Config.SteamOwnerID)

	// 加载插件（含内置示例插件与外部 .so 插件）
	if err := a.Plugins.LoadAll(); err != nil {
		return fmt.Errorf("插件加载失败: %w", err)
	}
	if err := a.loadExternalPlugins(); err != nil {
		a.Log.Warn("外部插件加载失败: %v", err)
	}
	cfgJSON, _ := json.Marshal(a.Config)
	if err := a.Plugins.NotifyASFInit(cfgJSON); err != nil {
		return fmt.Errorf("插件 ASF 初始化失败: %w", err)
	}

	if err := a.loadBots(); err != nil {
		return err
	}

	for _, b := range a.bots {
		go func(bot *bot.Bot) {
			if err := bot.Start(); err != nil {
				a.Log.Error("Bot %s 启动失败: %v", bot.Name, err)
			}
		}(b)
	}

	a.Log.Info("ASF 初始化完成，共 %d 个 Bot", len(a.bots))

	// 后台检查上游更新
	go a.CheckForUpdates()

	return nil
}

// CheckForUpdates 检查上游 ArchiSteamFarm 最新版本（用于演示更新机制）。
func (a *ASF) CheckForUpdates() {
	client := web.NewGitHubClient()
	release, err := client.GetLatestRelease("JustArchiNET", "ArchiSteamFarm", true)
	if err != nil {
		a.Log.Debug("更新检查失败: %v", err)
		return
	}
	a.Log.Info("上游 ArchiSteamFarm 最新版本: %s (发布于 %s)", release.TagName, release.PublishedAt)
	a.Log.Info("本项目为 Go 重写版 %s，版本号独立", Version)
}

// loadBots 扫描配置目录，加载所有 <BotName>.json 文件。
func (a *ASF) loadBots() error {
	entries, err := os.ReadDir(a.ConfigPath)
	if err != nil {
		return fmt.Errorf("读取配置目录失败: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if strings.EqualFold(entry.Name(), "ASF.json") {
			continue
		}
		botName := strings.TrimSuffix(entry.Name(), ".json")
		botPath := filepath.Join(a.ConfigPath, entry.Name())
		cfg, err := config.LoadBotConfig(botPath)
		if err != nil {
			a.Log.Warn("加载 Bot 配置 %s 失败: %v", botName, err)
			continue
		}
		b, err := bot.New(botName, cfg, a.ConfigPath)
		if err != nil {
			a.Log.Warn("创建 Bot %s 失败: %v", botName, err)
			continue
		}
		b.CommandPrefix = a.Config.CommandPrefix
		b.SteamAPIKey = a.Config.SteamWebApiKey
		b.OwnerSteamID = a.Config.SteamOwnerID
		b.ASFVersion = Version
		b.OnExit = func() { go func() { _ = a.Stop() }() }
		b.FindBotSteamID = func(name string) uint64 {
			for n, bb := range a.bots {
				if strings.EqualFold(n, name) {
					if bb.IsConnected() {
						return bb.SteamID()
					}
					return 0
				}
			}
			return 0
		}
		blacklist := make([]uint64, 0, len(a.Config.Blacklist))
		for _, id := range a.Config.Blacklist {
			blacklist = append(blacklist, uint64(id))
		}
		b.Blacklist = blacklist
		a.bots[botName] = b
		a.Log.Info("已加载 Bot 配置: %s (启用=%v)", botName, cfg.Enabled)
	}
	return nil
}

// StartTime 返回 ASF 进程启动时间。
func (a *ASF) StartTime() time.Time {
	return a.startTime
}

// UpdateGlobalConfig 更新全局配置并保存到 ASF.json（对应 IPC POST /api/ASF）。
func (a *ASF) UpdateGlobalConfig(cfg *config.GlobalConfig) error {
	if cfg == nil {
		return fmt.Errorf("全局配置为空")
	}
	a.Config = cfg
	return cfg.Save(filepath.Join(a.ConfigPath, "ASF.json"))
}

// UpdateBotConfig 更新指定 Bot 的配置并保存（对应 IPC POST /api/Bot/{name}）。
func (a *ASF) UpdateBotConfig(botName string, cfg *config.BotConfig) error {
	if botName == "" {
		return fmt.Errorf("Bot 名称为空")
	}
	if cfg == nil {
		return fmt.Errorf("Bot 配置为空")
	}
	b, ok := a.bots[botName]
	if !ok {
		return fmt.Errorf("Bot %s 不存在", botName)
	}
	b.Config = cfg
	return cfg.Save(filepath.Join(a.ConfigPath, botName+".json"))
}

// Stop 停止所有 Bot 并关闭日志。
func (a *ASF) Stop() error {
	a.Log.Info("ASF 正在关闭")
	for _, b := range a.bots {
		if err := b.Stop(); err != nil {
			a.Log.Warn("停止 Bot %s 失败: %v", b.Name, err)
		}
	}
	return a.Log.Close()
}

// RestartBots 重启所有 Bot 的挂机模块（对应 IPC /api/ASF/Restart）。
func (a *ASF) RestartBots() {
	for _, b := range a.bots {
		b.Pause()
		b.Resume()
	}
}

// Exit 触发 ASF 进程退出（对应 IPC /api/ASF/Exit）。
func (a *ASF) Exit() {
	if a.OnExit != nil {
		a.OnExit()
	} else {
		_ = a.Stop()
	}
}

// Update 检查 asf-go 自身更新并下载替换重启（对应 IPC /api/ASF/Update）。
func (a *ASF) Update() (string, error) {
	client := web.NewGitHubClient()
	release, err := client.GetLatestRelease("cyqmq", "asf-go", true)
	if err != nil {
		upstream, upErr := client.GetLatestRelease("JustArchiNET", "ArchiSteamFarm", true)
		if upErr != nil {
			return "", fmt.Errorf("检查更新失败: %w", err)
		}
		return fmt.Sprintf("asf-go 暂无 Release，上游 ASF 最新版本: %s", upstream.TagName), nil
	}

	if web.CompareVersions(release.TagName, Version) <= 0 {
		return fmt.Sprintf("当前已是最新版本 %s", Version), nil
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

	a.Log.Info("发现新版本 %s，正在下载 %s ...", release.TagName, asset.Name)
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

	a.Log.Info("更新完成，正在重启 ...")
	cmd := exec.Command(exe)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("重启失败: %w", err)
	}

	a.Exit()
	return fmt.Sprintf("已更新到 %s，正在重启", release.TagName), nil
}

// Bots 返回已加载的 Bot 映射。
func (a *ASF) Bots() map[string]*bot.Bot {
	return a.bots
}
