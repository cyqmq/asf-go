package core

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"

	"github.com/asf-go/asf/internal/plugins"
)

// loadExternalPlugins 扫描插件目录（GlobalConfig.PluginsDir）中的 .so 文件并动态加载。
// Go 插件通过导出 `var Plugins []plugins.Plugin` 符号注册自身（需与主程序同版本 Go 编译）。
func (a *ASF) loadExternalPlugins() error {
	dir := a.Config.PluginsDir
	if dir == "" {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取插件目录失败: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".so" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		p, err := plugin.Open(path)
		if err != nil {
			a.Log.Warn("加载插件 %s 失败: %v", entry.Name(), err)
			continue
		}
		sym, err := p.Lookup("Plugins")
		if err != nil {
			a.Log.Warn("插件 %s 缺少 Plugins 符号: %v", entry.Name(), err)
			continue
		}
		pluginsPtr, ok := sym.(*[]plugins.Plugin)
		if !ok {
			a.Log.Warn("插件 %s 的 Plugins 符号类型不匹配", entry.Name())
			continue
		}
		for _, pl := range *pluginsPtr {
			a.Plugins.Register(pl)
			a.Log.Info("已加载外部插件 %s v%s", pl.Name(), pl.Version())
		}
	}
	return nil
}
