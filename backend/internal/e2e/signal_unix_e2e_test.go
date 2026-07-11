//go:build e2e && !windows

package e2e_test

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func terminationSignal() os.Signal { return syscall.SIGTERM }
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGTERM, os.Interrupt)
}
