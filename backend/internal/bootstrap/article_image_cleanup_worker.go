package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const cleanupFailureMessage = "文章图片清理失败，将在下一轮重试"

// CleanupRunner 是 worker 调用的单轮图片清理能力。
type CleanupRunner interface {
	CleanupArticleImages(context.Context) error
}

// CleanupWorker 是 Bootstrap 持有的后台清理生命周期。
type CleanupWorker interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// cleanupTicker 抽象可停止的 tick 来源，供确定性测试注入。
type cleanupTicker interface {
	// C 返回 tick 事件通道。
	C() <-chan time.Time
	// Stop 释放 ticker 资源。
	Stop()
}

// systemCleanupTicker 适配标准库生产 ticker。
type systemCleanupTicker struct {
	// ticker 是唯一由 worker 停止的标准库资源。
	ticker *time.Ticker
}

// C 返回标准库 ticker 通道。
func (ticker systemCleanupTicker) C() <-chan time.Time { return ticker.ticker.C }

// Stop 停止标准库 ticker。
func (ticker systemCleanupTicker) Stop() { ticker.ticker.Stop() }

// articleImageCleanupWorker 串行执行清理并由 Bootstrap 显式停止。
type articleImageCleanupWorker struct {
	// runner 执行一轮协议无关清理。
	runner CleanupRunner
	// ticker 提供可合并的周期触发。
	ticker cleanupTicker
	// logger 记录可恢复的轮次失败。
	logger *slog.Logger
	// mutex 保护启动及停止句柄。
	mutex sync.Mutex
	// cancel 取消正在执行的清理。
	cancel context.CancelFunc
	// done 在 worker goroutine 完整退出后关闭。
	done chan struct{}
}

// NewArticleImageCleanupWorker 创建使用生产 ticker 的图片清理 worker。
func NewArticleImageCleanupWorker(runner CleanupRunner, interval time.Duration, logger *slog.Logger) (CleanupWorker, error) {
	if nilLike(runner) {
		return nil, errors.New("图片清理用例为空")
	}
	if interval <= 0 {
		return nil, errors.New("图片清理间隔必须大于零")
	}
	if logger == nil {
		return nil, errors.New("图片清理日志器为空")
	}
	return newCleanupWorker(runner, systemCleanupTicker{ticker: time.NewTicker(interval)}, logger), nil
}

// newCleanupWorker 使用显式 ticker 构造可确定性测试的 worker。
func newCleanupWorker(runner CleanupRunner, ticker cleanupTicker, logger *slog.Logger) *articleImageCleanupWorker {
	return &articleImageCleanupWorker{runner: runner, ticker: ticker, logger: logger}
}

// Start 启动唯一受管循环；重复调用保持幂等。
func (worker *articleImageCleanupWorker) Start(parent context.Context) error {
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.done != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	worker.cancel = cancel
	worker.done = make(chan struct{})
	go worker.run(ctx, worker.done)
	return nil
}

// run 串行消费 tick，直到生命周期 context 取消。
func (worker *articleImageCleanupWorker) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	defer worker.ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-worker.ticker.C():
			// 合并执行期间积压的 tick，确保同一 worker 永不重入。
			for draining := true; draining; {
				select {
				case <-worker.ticker.C():
				default:
					draining = false
				}
			}
			if err := worker.runner.CleanupArticleImages(ctx); err != nil && !errors.Is(err, context.Canceled) {
				worker.logger.Warn(cleanupFailureMessage, slog.Any("error", err))
			}
		}
	}
}

// Stop 取消进行中的清理并等待循环在调用方期限内退出。
func (worker *articleImageCleanupWorker) Stop(ctx context.Context) error {
	worker.mutex.Lock()
	if worker.done == nil {
		worker.mutex.Unlock()
		return nil
	}
	done, cancel := worker.done, worker.cancel
	worker.mutex.Unlock()
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
