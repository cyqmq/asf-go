// Package plugins 移植自 ArchiSteamFarm.Plugins，定义插件接口与管理器。
package plugins

import (
	"sort"
	"sync"
)

// Plugin 是所有插件的基础接口（对应 IPlugin）。
type Plugin interface {
	Name() string
	Version() string
	// OnLoaded 在插件加载完成后调用。
	OnLoaded() error
}

// ASFPlugin 可感知 ASF 全局配置（对应 IASF）。
type ASFPlugin interface {
	Plugin
	// OnASFInit 在 ASF 全局配置初始化后调用。configJSON 为全局配置 JSON。
	OnASFInit(configJSON []byte) error
}

// BotPlugin 可感知 Bot 生命周期（对应 IBot）。
type BotPlugin interface {
	Plugin
	// OnBotInit 在 Bot 创建后调用。botName 为 Bot 名称。
	OnBotInit(botName string) error
	// OnBotDestroy 在 Bot 销毁前调用。
	OnBotDestroy(botName string) error
}

// CommandPlugin 提供聊天命令（对应 IBotCommand2）。
type CommandPlugin interface {
	Plugin
	// ProcessCommand 处理聊天命令，返回是否已处理与回复文本。
	ProcessCommand(botName string, message string) (handled bool, reply string, err error)
}

// Manager 管理已注册插件。
type Manager struct {
	plugins []Plugin
	mu      sync.RWMutex
}

// NewManager 创建插件管理器。
func NewManager() *Manager {
	return &Manager{}
}

// Register 注册一个插件。
func (m *Manager) Register(p Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins = append(m.plugins, p)
}

// Plugins 返回已注册插件列表（按名称排序）。
func (m *Manager) Plugins() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Plugin, len(m.plugins))
	copy(out, m.plugins)
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// LoadAll 调用所有插件的 OnLoaded。
func (m *Manager) LoadAll() error {
	for _, p := range m.Plugins() {
		if err := p.OnLoaded(); err != nil {
			return err
		}
	}
	return nil
}

// NotifyASFInit 通知所有 ASF 插件全局配置已初始化。
func (m *Manager) NotifyASFInit(configJSON []byte) error {
	for _, p := range m.Plugins() {
		if ap, ok := p.(ASFPlugin); ok {
			if err := ap.OnASFInit(configJSON); err != nil {
				return err
			}
		}
	}
	return nil
}

// NotifyBotInit 通知所有 Bot 插件 Bot 已创建。
func (m *Manager) NotifyBotInit(botName string) error {
	for _, p := range m.Plugins() {
		if bp, ok := p.(BotPlugin); ok {
			if err := bp.OnBotInit(botName); err != nil {
				return err
			}
		}
	}
	return nil
}

// NotifyBotDestroy 通知所有 Bot 插件 Bot 即将销毁。
func (m *Manager) NotifyBotDestroy(botName string) error {
	for _, p := range m.Plugins() {
		if bp, ok := p.(BotPlugin); ok {
			if err := bp.OnBotDestroy(botName); err != nil {
				return err
			}
		}
	}
	return nil
}
