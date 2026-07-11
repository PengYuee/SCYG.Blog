//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSignalChildProcess 是 E2E 测试二进制的真实 API 子进程入口。
func TestSignalChildProcess(t *testing.T) {
	marker := os.Getenv("SCYG_E2E_SIGNAL_MARKER")
	if marker == "" {
		return
	}
	h := newHarness(t, nil)
	pool := openPool(t, h.dsn)
	if err := os.WriteFile(marker+".ready", []byte(h.baseURL), 0o600); err != nil {
		t.Fatalf("写入子进程就绪标记失败：%v", err)
	}
	signalCtx, stop := signalContext(context.Background())
	defer stop()
	if err := h.app.Run(signalCtx); err != nil {
		t.Fatalf("SIGTERM 关闭应用失败：%v", err)
	}
	h.app = nil
	if err := pool.Close(); err != nil {
		t.Fatalf("关闭子进程 SQL pool 失败：%v", err)
	}
	if err := pool.PingContext(context.Background()); !errors.Is(err, os.ErrClosed) && err == nil {
		t.Fatal("SQLDB 关闭后仍可 Ping")
	}
	if err := os.WriteFile(marker+".closed", []byte("telemetry=shutdown\ndatabase=closed"), 0o600); err != nil {
		t.Fatalf("写入关闭标记失败：%v", err)
	}
	h.close()
}

// assertSignalSubprocessShutdown 向真实测试 API 子进程发送平台信号并验证各资源结果。
func assertSignalSubprocessShutdown(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	marker := filepath.Join(t.TempDir(), "signal")
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestSignalChildProcess", "-test.count=1")
	command.Env = append(os.Environ(), "SCYG_E2E_SIGNAL_MARKER="+marker)
	if err := command.Start(); err != nil {
		t.Fatalf("启动 E2E 信号子进程失败：%v", err)
	}
	ready := waitMarker(t, ctx, marker+".ready")
	baseURL := strings.TrimSpace(string(ready))
	if err := command.Process.Signal(terminationSignal()); err != nil {
		t.Fatalf("发送终止信号失败：%v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("等待信号子进程失败：%v", err)
	}
	closed := string(waitMarker(t, ctx, marker+".closed"))
	if !strings.Contains(closed, "telemetry=shutdown") || !strings.Contains(closed, "database=closed") {
		t.Fatalf("关闭标记不完整：%s", closed)
	}
	client := &http.Client{Timeout: time.Second}
	if _, err := client.Get(baseURL + "/ready"); err == nil {
		t.Fatal("SIGTERM 后 HTTP/readiness 仍可访问")
	}
}

func waitMarker(t *testing.T, ctx context.Context, path string) []byte {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		content, err := os.ReadFile(path)
		if err == nil {
			return content
		}
		select {
		case <-ctx.Done():
			t.Fatalf("等待子进程标记超时：%v", ctx.Err())
		case <-ticker.C:
		}
	}
}
