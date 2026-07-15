//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// lifecycleSnapshot 是由 bootstrap 实际关闭回调形成的可序列化状态。
type lifecycleSnapshot struct {
	// ReadinessWithdrawn 表示 App 已撤回 readiness。
	ReadinessWithdrawn bool
	// HTTPClosed 表示 App-owned HTTP 已关闭。
	HTTPClosed bool
	// WorkerStopped 表示图片清理 worker 已停止。
	WorkerStopped bool
	// DatabaseClosed 表示 App-owned database 已关闭。
	DatabaseClosed bool
	// TelemetryClosed 表示 App-owned telemetry 已关闭。
	TelemetryClosed bool
}

// lifecycleSnapshotObserver 并发安全记录 App-owned 资源关闭事实。
type lifecycleSnapshotObserver struct {
	// mutex 保护关闭回调和快照读取。
	mutex sync.Mutex
	// snapshot 只由 bootstrap observer 回调更新。
	snapshot lifecycleSnapshot
}

func (observer *lifecycleSnapshotObserver) ReadinessWithdrawn() {
	observer.mutex.Lock()
	defer observer.mutex.Unlock()
	observer.snapshot.ReadinessWithdrawn = true
}

func (observer *lifecycleSnapshotObserver) HTTPClosed() {
	observer.mutex.Lock()
	defer observer.mutex.Unlock()
	observer.snapshot.HTTPClosed = true
}

// WorkerStopped 记录图片清理 worker 已确认退出。
func (observer *lifecycleSnapshotObserver) WorkerStopped() {
	observer.mutex.Lock()
	defer observer.mutex.Unlock()
	observer.snapshot.WorkerStopped = true
}

func (observer *lifecycleSnapshotObserver) DatabaseClosed() {
	observer.mutex.Lock()
	defer observer.mutex.Unlock()
	observer.snapshot.DatabaseClosed = true
}

func (observer *lifecycleSnapshotObserver) TelemetryClosed() {
	observer.mutex.Lock()
	defer observer.mutex.Unlock()
	observer.snapshot.TelemetryClosed = true
}

func (observer *lifecycleSnapshotObserver) Snapshot() lifecycleSnapshot {
	observer.mutex.Lock()
	defer observer.mutex.Unlock()
	return observer.snapshot
}

// TestSignalChildProcess 是 E2E 测试二进制的真实 API 子进程入口。
func TestSignalChildProcess(t *testing.T) {
	marker := os.Getenv("SCYG_E2E_SIGNAL_MARKER")
	if marker == "" {
		return
	}
	observer := &lifecycleSnapshotObserver{}
	h := newHarnessWithObserver(t, nil, observer)
	if err := os.WriteFile(marker+".ready", []byte(h.baseURL), 0o600); err != nil {
		t.Fatalf("写入子进程就绪标记失败：%v", err)
	}
	signalCtx, stop := signalContext(context.Background(), marker)
	defer stop()
	if err := h.app.Run(signalCtx); err != nil {
		t.Fatalf("SIGTERM 关闭应用失败：%v", err)
	}
	h.app = nil
	snapshot, err := json.Marshal(observer.Snapshot())
	if err != nil {
		t.Fatalf("序列化真实生命周期快照失败：%v", err)
	}
	if err = os.WriteFile(marker+".closed", snapshot, 0o600); err != nil {
		t.Fatalf("写入真实关闭快照失败：%v", err)
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
	if err := requestGracefulStop(command.Process, marker); err != nil {
		t.Fatalf("发送终止信号失败：%v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("等待信号子进程失败：%v", err)
	}
	closed := waitMarker(t, ctx, marker+".closed")
	var snapshot lifecycleSnapshot
	if err := json.Unmarshal(closed, &snapshot); err != nil {
		t.Fatalf("解析真实生命周期快照失败：%v", err)
	}
	if !snapshot.ReadinessWithdrawn || !snapshot.HTTPClosed || !snapshot.WorkerStopped || !snapshot.DatabaseClosed || !snapshot.TelemetryClosed {
		t.Fatalf("真实生命周期快照不完整：%+v", snapshot)
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
