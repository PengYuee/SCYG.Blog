package content

import (
	"errors"
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// ErrorCode 是与协议无关的稳定应用错误码；不得翻译或更改其字符串值。
type ErrorCode string

// ErrorKind 按调用方处理方式归类应用错误。
type ErrorKind string

const (
	CodeValidation         ErrorCode = "validation"
	CodePermissionDenied   ErrorCode = "permission_denied"
	CodeNotFound           ErrorCode = "not_found"
	CodeAlreadyExists      ErrorCode = "already_exists"
	CodeFailedPrecondition ErrorCode = "failed_precondition"
	CodeVersionRequired    ErrorCode = "version_required"
	CodeStaleVersion       ErrorCode = "stale_version"
	CodeInternal           ErrorCode = "internal"
	KindValidation         ErrorKind = "validation"
	KindPermission         ErrorKind = "permission"
	KindMissing            ErrorKind = "missing"
	KindConflict           ErrorKind = "conflict"
	KindInternal           ErrorKind = "internal"
)

// 稳定应用错误哨兵仅表达业务语义，不暴露适配器底层原因。
var (
	// ErrConflict 表示当前资源状态与操作冲突。
	ErrConflict = errors.New("内容：资源状态冲突")
	// ErrFailedPrecondition 表示操作的业务前置条件未满足。
	ErrFailedPrecondition = errors.New("内容：前置条件不满足")
	// ErrNotFound 表示请求资源不存在。
	ErrNotFound = errors.New("内容：资源不存在")
	// ErrPersistence 表示持久化失败；不得将底层原因返回给接口调用方。
	ErrPersistence = errors.New("内容：持久化失败")
)

// ErrPermissionDenied 是稳定的授权拒绝哨兵。
var ErrPermissionDenied = errors.New("内容：权限不足")

// ApplicationError 携带不绑定 HTTP 状态码的稳定错误语义；Cause 仅用于内部错误链。
type ApplicationError struct {
	// Code 是面向调用方的稳定机器错误码。
	Code ErrorCode
	// Kind 是供应用层分支处理的错误分类。
	Kind ErrorKind
	// Cause 是内部根因，HTTP 响应不得直接泄露它。
	Cause error
	// ExpectedVersion 是并发冲突中的期望版本。
	ExpectedVersion uint64
	// ActualVersion 是并发冲突中持久化的实际版本。
	ActualVersion uint64
}

// Error 返回稳定错误码与中文原因，供日志和内部调用链使用。
func (failure *ApplicationError) Error() string {
	return fmt.Sprintf("%s: %v", failure.Code, failure.Cause)
}

// Unwrap 保留 errors.Is 和 errors.As 所需的错误链。
func (failure *ApplicationError) Unwrap() error { return failure.Cause }

func validation(err error) error {
	return &ApplicationError{Code: CodeValidation, Kind: KindValidation, Cause: err}
}
func permission(err error) error {
	if errors.Is(err, ErrPermissionDenied) {
		return err
	}
	return &ApplicationError{Code: CodePermissionDenied, Kind: KindPermission, Cause: ErrPermissionDenied}
}
func stable(err error) error {
	if err == nil {
		return nil
	}
	var known *ApplicationError
	if errors.As(err, &known) {
		return known
	}
	var conflict *domain.VersionConflict
	if errors.As(err, &conflict) {
		return &ApplicationError{Code: CodeStaleVersion, Kind: KindConflict, Cause: domain.ErrStaleVersion, ExpectedVersion: conflict.Expected.Uint64(), ActualVersion: conflict.Actual.Uint64()}
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return &ApplicationError{Code: CodeNotFound, Kind: KindMissing, Cause: ErrNotFound}
	case errors.Is(err, ErrConflict):
		return &ApplicationError{Code: CodeAlreadyExists, Kind: KindConflict, Cause: ErrConflict}
	case errors.Is(err, ErrFailedPrecondition), errors.Is(err, domain.ErrNoChange), errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrDeleted):
		return &ApplicationError{Code: CodeFailedPrecondition, Kind: KindConflict, Cause: ErrFailedPrecondition}
	case errors.Is(err, domain.ErrInvalidValue), errors.Is(err, domain.ErrDuplicateTag):
		return validation(err)
	default:
		return &ApplicationError{Code: CodeInternal, Kind: KindInternal, Cause: ErrPersistence}
	}
}

func invalidCommand(field string) error { return fmt.Errorf("%s: %w", field, domain.ErrInvalidValue) }
