//go:build windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

func configureChild(_ *exec.Cmd) error { return nil }

func terminateChild(ctx context.Context, cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	//nolint:gosec // taskkill targets the process started by this QA runner.
	kill := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	if output, err := kill.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill: %w: %s", err, output)
	}
	return nil
}
