package ipc

import "embed"

// uiFS 嵌入 ASF-ui 的静态文件（官方 Web 界面，Apache-2.0）。
//
//go:embed ui/www
var uiFS embed.FS
