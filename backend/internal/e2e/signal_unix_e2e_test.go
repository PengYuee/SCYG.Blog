//go:build e2e && !windows

package e2e_test

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func requestGracefulStop(process *os.Process, _ string) error { return process.Signal(syscall.SIGTERM) }
func signalContext(parent context.Context, _ string) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGTERM, os.Interrupt)
}

func terminationUsesNativeSignal() bool { return true }
