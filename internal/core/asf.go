// Package core 移植自 ArchiSteamFarm.Core.ASF（C# 主入口的精简版）。
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	bots map[string]*bot.Bot
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

	// 加载插件（含内置示例插件）
	if err := a.Plugins.LoadAll(); err != nil {
		return fmt.Errorf("插件加载失败: %w", err)
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
		a.bots[botName] = b
		a.Log.Info("已加载 Bot 配置: %s (启用=%v)", botName, cfg.Enabled)
	}
	return nil
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

// Bots 返回已加载的 Bot 映射。
func (a *ASF) Bots() map[string]*bot.Bot {
	return a.bots
}