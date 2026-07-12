package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidValue        = errors.New("内容领域：值不合法")
	ErrInvalidTransition   = errors.New("内容领域：状态迁移不合法")
	ErrDuplicateTag        = errors.New("内容领域：标签重复")
	ErrArticleTypeRequired = errors.New("内容领域：必须指定文章分类")
	ErrContentRequired     = errors.New("内容领域：必须提供正文")
	ErrStaleVersion        = errors.New("内容领域：版本已过期")
	// ErrNoChange 表示修订后的状态与当前状态相同。
	ErrNoChange = errors.New("内容领域：没有变更")
	// ErrVersionExhausted 表示聚合版本无法再安全递增。
	ErrVersionExhausted = errors.New("内容领域：版本号已耗尽")
	// ErrInvalidClock 表示领域时钟为 nil 或零值。
	ErrInvalidClock = errors.New("内容领域：时钟无效")
	// ErrTimeRegression 表示变更时间早于此前的领域时间。
	ErrTimeRegression = errors.New("内容领域：时间倒退")
	// ErrDeleted 表示对已软删除实体执行了操作。
	ErrDeleted = errors.New("内容领域：实体已删除")
)

// VersionConflict 描述乐观锁冲突中的期望版本和实际版本。
type VersionConflict struct {
	// Expected 是调用方提交的期望版本。
	Expected Version
	// Actual 是当前持久化版本。
	Actual Version
}

// Error 返回不包含持久化细节的中文版本冲突描述。
func (conflict *VersionConflict) Error() string {
	return fmt.Sprintf("%v：期望版本 %d，实际版本 %d", ErrStaleVersion, conflict.Expected.Uint64(), conflict.Actual.Uint64())
}

// Unwrap 允许调用方以 ErrStaleVersion 判断版本冲突。
func (*VersionConflict) Unwrap() error { return ErrStaleVersion }

func invalid(field string) error { return fmt.Errorf("%s: %w", field, ErrInvalidValue) }
