//go:build e2e && windows

package e2e_test

import (
	"context"
	"os"
	"time"
)

// requestGracefulStop 使用测试专用控制文件请求子进程走 App.Run 的真实关闭路径。
func requestGracefulStop(_ *os.Process, marker string) error {
	return os.WriteFile(marker+".stop", []byte("graceful"), 0o600)
}

// signalContext 在 Windows 轮询 localhost-only 测试控制文件并转换为 context cancellation。
func signalContext(parent context.Context, marker string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, err := os.Stat(marker + ".stop"); err == nil {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return ctx, cancel
}

func terminationUsesNativeSignal() bool { return false }
