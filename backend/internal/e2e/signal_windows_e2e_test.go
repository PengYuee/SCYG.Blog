//go:build e2e && windows

package e2e_test

import (
	"context"
	"os"
	"os/signal"
)

// Windows 无 SIGTERM，使用 os.Interrupt 验证同一优雅关闭语义。
func terminationSignal() os.Signal { return os.Interrupt }
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt)
}
