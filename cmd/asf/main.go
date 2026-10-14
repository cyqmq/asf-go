// ASF Go 版入口。用法：asf --config <配置目录>
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/asf-go/asf/internal/core"
	"github.com/asf-go/asf/internal/ipc"
	"github.com/asf-go/asf/internal/logging"
)

func main() {
	configPath := flag.String("config", "config", "ASF 配置目录（包含 ASF.json）")
	ipcAddr := flag.String("ipc-addr", ":1242", "IPC 服务监听地址")
	flag.Parse()

	asf, err := core.New(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		os.Exit(1)
	}

	if err := asf.Start(); err != nil {
		asf.Log.Error("启动失败: %v", err)
		asf.Stop()
		os.Exit(1)
	}

	var ipcSrv *ipc.Server
	var ipcLog *logging.Logger
	if asf.Config.IPC {
		ipcLog, err = logging.New("IPC", *configPath+"/asf-go.log", logging.Info)
		if err != nil {
			asf.Log.Error("创建 IPC 日志器失败: %v", err)
		} else {
			ipcSrv = ipc.New(asf, ipcLog, asf.Config.IPCPassword)
			if err := ipcSrv.Start(*ipcAddr); err != nil {
				asf.Log.Error("启动 IPC 失败: %v", err)
			}
		}
	}

	// IPC /api/ASF/Exit 触发时优雅退出
	asf.OnExit = func() {
		if ipcSrv != nil {
			_ = ipcSrv.Stop()
		}
		if ipcLog != nil {
			_ = ipcLog.Close()
		}
		_ = asf.Stop()
		os.Exit(0)
	}

	// 等待退出信号
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	asf.Log.Info("收到信号 %v", s)
	if ipcSrv != nil {
		_ = ipcSrv.Stop()
	}
	if ipcLog != nil {
		_ = ipcLog.Close()
	}
	if err := asf.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "关闭失败: %v\n", err)
		os.Exit(1)
	}
}
