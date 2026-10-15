# ASF-Go 插件开发指南

ASF-Go 支持通过 Go 的 `plugin` 包（`.so` 动态库）加载第三方插件，接口定义在 `internal/plugins`。

## 插件接口

### 基础接口

```go
type Plugin interface {
	Name() string
	Version() string
	OnLoaded() error
}
```

### 可选扩展接口

| 接口 | 说明 |
|------|------|
| `ASFPlugin` | 感知 ASF 全局配置，`OnASFInit(configJSON []byte) error` 在全局配置初始化后调用 |
| `BotPlugin` | 感知 Bot 生命周期，`OnBotInit(botName string)` / `OnBotDestroy(botName string)` |
| `CommandPlugin` | 提供聊天命令，`ProcessCommand(botName string, message string) (handled bool, reply string, err error)` |

一个插件可以实现多个接口。

## 编写第一个插件

参考 `examples/plugins/hello`：

```go
package main

import "github.com/asf-go/asf/internal/plugins"

var Plugins = []plugins.Plugin{&helloPlugin{}}

type helloPlugin struct{}

func (p *helloPlugin) Name() string    { return "HelloPlugin" }
func (p *helloPlugin) Version() string  { return "1.0.0" }
func (p *helloPlugin) OnLoaded() error { return nil }
```

注意：
- 插件必须是 `package main`。
- 必须导出 `var Plugins = []plugins.Plugin{...}`（符号名固定为 `Plugins`）。
- **必须包含 `func main() {}`**（Go `-buildmode=plugin` 的要求）。

## 构建与配置

```bash
# 构建 .so
go build -buildmode=plugin -o hello.so ./examples/plugins/hello
```

在全局配置 `ASF.json` 中指定插件目录（默认 `plugins`）：

```json
{
  "PluginsDir": "plugins"
}
```

将 `hello.so` 放入该目录，启动 ASF-Go 即会自动加载。IPC 端点 `GET /api/plugins` 可查看已加载插件。

## 测试插件

```bash
go test ./internal/core/ -run TestLoadExternalPlugin -v
```

该测试会现场编译示例插件并验证加载。

## 从原版 ASF 插件迁移

原版 ASF 插件是 C#/.NET 程序集，无法直接复用。迁移时需要：

1. 理解原版插件调用的 ASF API（`IBot`、`IASF`、`IBotCommand` 等）。
2. 在 Go 侧找到对应能力：
   - 全局配置 → `core.ASF.Config`
   - Bot 生命周期 → `BotPlugin.OnBotInit/OnBotDestroy`
   - 聊天命令 → `CommandPlugin.ProcessCommand`
   - 交易/挂机事件 → 当前版本尚未暴露，可先通过 Bot 内部接口扩展
3. 用 Go 重写插件逻辑并导出 `Plugins` 符号。

> 注意：Go 插件（`.so`）与主程序必须使用**相同的 Go 版本**构建，且需与主程序运行时一致（如 `-race` 标志）。升级 Go 版本后需要重新构建插件。