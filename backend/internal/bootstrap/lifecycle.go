package bootstrap

// LifecycleObserver 接收 App 已完成的生命周期事实；实现必须并发安全且不得反向控制资源。
type LifecycleObserver interface {
	// ReadinessWithdrawn 记录 readiness 已由 App 撤回。
	ReadinessWithdrawn()
	// HTTPClosed 记录 App-owned HTTP server 已成功关闭。
	HTTPClosed()
	// WorkerStopped 记录图片清理 worker 已停止。
	WorkerStopped()
	// DatabaseClosed 记录 App-owned database 已成功关闭。
	DatabaseClosed()
	// TelemetryClosed 记录 App-owned telemetry 已成功关闭。
	TelemetryClosed()
}

// noopLifecycleObserver 是生产默认观察器，不引入全局状态。
type noopLifecycleObserver struct{}

func (noopLifecycleObserver) ReadinessWithdrawn() {}
func (noopLifecycleObserver) HTTPClosed()         {}
func (noopLifecycleObserver) WorkerStopped()      {}
func (noopLifecycleObserver) DatabaseClosed()     {}
func (noopLifecycleObserver) TelemetryClosed()    {}

// lifecycleObserverOrDefault 保留显式观察器，并为 nil 提供无状态默认实现。
func lifecycleObserverOrDefault(observer LifecycleObserver) LifecycleObserver {
	if observer == nil {
		return noopLifecycleObserver{}
	}
	return observer
}
