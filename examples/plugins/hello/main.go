// 示例 Go 插件：演示 ASF-Go 动态插件加载。
// 编译为 .so 后放入 GlobalConfig.PluginsDir 指定目录：
//
//	go build -buildmode=plugin -o plugins/hello.so ./examples/plugins/hello
package main

import "github.com/asf-go/asf/internal/plugins"

type helloPlugin struct{}

func (helloPlugin) Name() string    { return "HelloPlugin" }
func (helloPlugin) Version() string { return "1.0.0" }

func (helloPlugin) OnLoaded() error {
	return nil
}

// Plugins 是 ASF-Go 动态加载的导出符号，类型必须为 []plugins.Plugin。
var Plugins = []plugins.Plugin{helloPlugin{}}

func main() {}
