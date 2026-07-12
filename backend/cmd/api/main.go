// Package main 是 API 进程薄入口。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
)

// main 仅解析配置路径、建立信号上下文并把生命周期交给 bootstrap。
func main() {
	configFile, err := parseConfigFile(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app, err := bootstrap.New(ctx, bootstrap.Options{ConfigFile: configFile, LogWriter: os.Stderr}, bootstrap.DefaultDependencies())
	if err == nil {
		err = app.Run(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "API 服务启动或运行失败")
		os.Exit(1)
	}
}
