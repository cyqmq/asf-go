package core

import (
	"github.com/asf-go/asf/internal/logging"
)

// builtinPlugin 是内置示例插件，演示插件生命周期回调。
type builtinPlugin struct {
	log *logging.Logger
}

func (p *builtinPlugin) Name() string    { return "BuiltinExample" }
func (p *builtinPlugin) Version() string { return "1.0.0" }

func (p *builtinPlugin) OnLoaded() error {
	p.log.Info("内置插件 %s v%s 已加载", p.Name(), p.Version())
	return nil
}

// registerBuiltinPlugins 注册内置插件。
func (a *ASF) registerBuiltinPlugins() {
	a.Plugins.Register(&builtinPlugin{log: a.Log})
}