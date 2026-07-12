package content

import (
	"context"
	"reflect"
)

// Action 表示与传输协议无关的授权动作；其字符串值是稳定权限契约。
type Action string

const (
	// ActionCreateArticle 授权创建文章草稿。
	ActionCreateArticle Action = "content.article.create"
	// ActionReviseArticle 授权修订文章。
	ActionReviseArticle Action = "content.article.revise"
	// ActionPublishArticle 授权发布文章。
	ActionPublishArticle Action = "content.article.publish"
	// ActionArchiveArticle 授权归档文章。
	ActionArchiveArticle Action = "content.article.archive"
	// ActionDeleteArticle 授权删除文章。
	ActionDeleteArticle Action = "content.article.delete"
	// ActionManageArticleType 授权变更文章分类。
	ActionManageArticleType Action = "content.article_type.manage"
	// ActionManageTag 授权变更标签。
	ActionManageTag Action = "content.tag.manage"
)

// Resource 描述待授权的领域资源；Kind 和 ID 必须与鉴权策略使用的资源标识一致。
type Resource struct {
	// Kind 是资源类别的稳定标识。
	Kind string
	// ID 是资源的业务主键。
	ID int64
}

// Authorizer 决定主体是否可对指定资源执行动作；拒绝时必须返回权限错误。
type Authorizer interface {
	Authorize(context.Context, Action, Resource) error
}

// AuthorizerOrDeny 保留有效鉴权器，并将 nil 接口或带类型 nil 安全地降级为 DenyAll，确保默认拒绝。
func AuthorizerOrDeny(candidate Authorizer) Authorizer {
	value := reflect.ValueOf(candidate)
	if !value.IsValid() {
		return DenyAll{}
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return DenyAll{}
		}
	}
	return candidate
}

// DenyAll 拒绝全部授权请求，是缺省鉴权器的安全实现。
type DenyAll struct{}

// Authorize 始终返回 permission_denied，不允许缺省配置绕过授权。
func (DenyAll) Authorize(context.Context, Action, Resource) error {
	return &ApplicationError{Code: CodePermissionDenied, Kind: KindPermission, Cause: ErrPermissionDenied}
}
