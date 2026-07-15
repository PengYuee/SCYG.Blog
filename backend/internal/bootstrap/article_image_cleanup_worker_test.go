package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type cleanupRunnerFake struct {
	// mutex 保护并发统计。
	mutex sync.Mutex
	// calls 记录总调用次数。
	calls int
	// active 记录当前执行数。
	active int
	// maximum 记录最大并发数。
	maximum int
	// started 通知测试一轮已进入。
	started chan struct{}
	// release 控制首轮释放。
	release chan struct{}
	// errors 按调用顺序返回故障。
	errors []error
}

func (runner *cleanupRunnerFake) CleanupArticleImages(ctx context.Context) error {
	runner.mutex.Lock()
	runner.calls++
	runner.active++
	if runner.active > runner.maximum {
		runner.maximum = runner.active
	}
	call := runner.calls
	runner.mutex.Unlock()
	defer func() {
		runner.mutex.Lock()
		runner.active--
		runner.mutex.Unlock()
	}()
	if runner.started != nil {
		select {
		case runner.started <- struct{}{}:
		default:
		}
	}
	if runner.release != nil {
		select {
		case <-runner.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if call <= len(runner.errors) {
		return runner.errors[call-1]
	}
	return nil
}

// maximumConcurrency 返回清理执行期间观测到的最大并发数。
func (runner *cleanupRunnerFake) maximumConcurrency() int {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()
	return runner.maximum
}

// callCount 返回累计清理次数。
func (runner *cleanupRunnerFake) callCount() int {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()
	return runner.calls
}

// cleanupTickerFake 提供测试可控 tick。
type cleanupTickerFake struct {
	// ticks 是测试注入的触发通道。
	ticks chan time.Time
}

func (ticker *cleanupTickerFake) C() <-chan time.Time { return ticker.ticks }
func (*cleanupTickerFake) Stop()                      {}

func Test_CleanupWorker_coalesces_ticks_while_cleanup_is_running(t *testing.T) {
	// Given
	runner := &cleanupRunnerFake{started: make(chan struct{}, 2), release: make(chan struct{})}
	ticker := &cleanupTickerFake{ticks: make(chan time.Time, 8)}
	worker := newCleanupWorker(runner, ticker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = worker.Stop(context.Background()) }()

	// When
	ticker.ticks <- time.Now()
	<-runner.started
	ticker.ticks <- time.Now()
	ticker.ticks <- time.Now()
	select {
	case <-runner.started:
		t.Fatal("首轮释放前启动了重入清理")
	default:
	}
	close(runner.release)
	<-runner.started

	// Then
	if got, maximum := runner.callCount(), runner.maximumConcurrency(); got != 2 || maximum != 1 {
		t.Fatalf("重复 tick 未正确串行合并：calls=%d maximum=%d", got, maximum)
	}
}

func Test_CleanupWorker_recovers_on_next_tick_after_error(t *testing.T) {
	// Given
	runner := &cleanupRunnerFake{started: make(chan struct{}, 2), errors: []error{errors.New("本轮失败")}}
	ticker := &cleanupTickerFake{ticks: make(chan time.Time, 2)}
	worker := newCleanupWorker(runner, ticker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = worker.Stop(context.Background()) }()

	// When
	ticker.ticks <- time.Now()
	<-runner.started
	ticker.ticks <- time.Now()
	<-runner.started

	// Then
	if got := runner.callCount(); got != 2 {
		t.Fatalf("失败后未恢复：calls=%d", got)
	}
}

func Test_CleanupWorker_stop_cancels_inflight_cleanup(t *testing.T) {
	// Given
	runner := &cleanupRunnerFake{started: make(chan struct{}, 1), release: make(chan struct{})}
	ticker := &cleanupTickerFake{ticks: make(chan time.Time, 1)}
	worker := newCleanupWorker(runner, ticker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ticker.ticks <- time.Now()
	<-runner.started

	// When
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := worker.Stop(ctx)
	// Then
	if err != nil {
		t.Fatalf("worker 未有界停止：%v", err)
	}
}
