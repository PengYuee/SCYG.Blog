package content

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
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

// CreateArticle 描述创建文章草稿所需的已解析输入。
type CreateArticle struct {
	// ArticleTypeID 是所属文章分类 ID。
	ArticleTypeID int64
	// Title 是文章标题。
	Title string
	// Slug 是文章的 URL 标识。
	Slug string
	// Digest 是文章摘要。
	Digest string
	// Content 是文章正文。
	Content string
	// TagIDs 是文章关联的标签 ID。
	TagIDs []int64
}

// ReviseArticle 描述基于乐观锁版本修订文章的完整输入。
type ReviseArticle struct {
	// ID 是待修订文章 ID。
	ID int64
	// Version 是调用方持有的乐观锁版本。
	Version uint64
	// ArticleTypeID 是目标文章分类 ID。
	ArticleTypeID int64
	// Title 是目标文章标题。
	Title string
	// Slug 是目标 URL 标识。
	Slug string
	// Digest 是目标摘要。
	Digest string
	// Content 是目标正文。
	Content string
	// TagIDs 是目标标签 ID 集合。
	TagIDs []int64
}

// PatchArticle 描述文章局部更新；nil 字段表示调用方未提供，Version 必须来自强 ETag。
type PatchArticle struct {
	// ID 是待更新文章 ID。
	ID int64
	// Version 是 If-Match 解析出的乐观锁版本。
	Version uint64
	// ArticleTypeID 为可选目标分类 ID。
	ArticleTypeID *int64
	// Title 为可选标题。
	Title *string
	// Slug 为可选 URL 标识。
	Slug *string
	// Digest 为可选摘要。
	Digest *string
	// Content 为可选正文。
	Content *string
	// TagIDs 为可选标签集合，非 nil 的空切片表示清空。
	TagIDs *[]int64
	// Status 为可选生命周期状态。
	Status *string
}

// PublishArticle 描述基于版本发布草稿的命令。
type PublishArticle struct {
	// ID 是待发布文章 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}

// ArchiveArticle 描述基于版本归档已发布文章的命令。
type ArchiveArticle struct {
	// ID 是待归档文章 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}

// DeleteArticle 描述基于版本软删除文章的命令。
type DeleteArticle struct {
	// ID 是待删除文章 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}

// CreateArticleType 描述创建文章分类的输入；Meun 必须为非负值。
type CreateArticleType struct {
	// Name 是分类名称。
	Name string
	// Image 是可选分类图片。
	Image *string
	// Meun 是稳定的菜单排序字段名。
	Meun int32
}

// OptionalImage 区分未提供图片、设置图片和显式清空图片。
type OptionalImage struct {
	// Provided 表示请求是否包含 image 字段。
	Provided bool
	// Value 是提供的图片值，nil 表示显式清空。
	Value *string
}

// PatchArticleType 描述基于 ETag 版本局部更新文章分类的命令。
type PatchArticleType struct {
	// ID 是待更新分类 ID。
	ID int64
	// Version 是 If-Match 解析出的乐观锁版本。
	Version uint64
	// Name 是可选分类名称。
	Name *string
	// Image 保留 image 的三态语义。
	Image OptionalImage
	// Meun 是可选菜单排序。
	Meun *int32
}

// RenameArticleType 描述基于版本重命名文章分类的命令。
type RenameArticleType struct {
	// ID 是待重命名分类 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
	// Name 是新的分类名称。
	Name string
}

// DeleteArticleType 描述基于版本软删除文章分类的命令。
type DeleteArticleType struct {
	// ID 是待删除分类 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}

// CreateTag 描述创建标签的输入。
type CreateTag struct {
	// Name 是标签名称。
	Name string
}

// RenameTag 描述基于版本重命名标签的命令。
type RenameTag struct {
	// ID 是待重命名标签 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
	// Name 是新的标签名称。
	Name string
}

// DeleteTag 描述基于版本软删除标签的命令。
type DeleteTag struct {
	// ID 是待删除标签 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}

// GetArticle 描述查询一篇公开文章的输入。
type GetArticle struct {
	// ID 是文章 ID。
	ID int64
}

// ListArticles 描述公开文章的分页与筛选条件，调用方必须提供已受限的页码参数。
type ListArticles struct {
	// Page 是从 1 开始的页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// ArticleTypeID 是可选分类筛选。
	ArticleTypeID int64
	// TagID 是可选标签筛选。
	TagID int64
	// Query 是可选检索词。
	Query string
	// Sort 是稳定排序标识。
	Sort string
}

// ListArticleTypes 描述文章分类的协议无关筛选和分页条件。
type ListArticleTypes struct {
	// Page 是从 1 开始的页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// Name 是可选名称筛选。
	Name string
	// Sort 是稳定排序标识。
	Sort string
}

// GetArticleType 描述查询一个未删除文章分类的输入。
type GetArticleType struct {
	// ID 是文章分类 ID。
	ID int64
}

// ListTags 描述标签的协议无关筛选和分页条件。
type ListTags struct {
	// Page 是从 1 开始的页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// Name 是可选名称筛选。
	Name string
	// Sort 是稳定排序标识。
	Sort string
}

// GetTag 描述查询一个未删除标签的输入。
type GetTag struct {
	// ID 是标签 ID。
	ID int64
}

// ArticleResult 是协议无关的文章读取结果，所有文本已通过领域规则校验。
type ArticleResult struct {
	// ID 是文章 ID。
	ID int64
	// ArticleTypeID 是所属分类 ID。
	ArticleTypeID int64
	// Title 是文章标题。
	Title string
	// Slug 是文章 URL 标识。
	Slug string
	// Digest 是文章摘要。
	Digest string
	// Content 是文章正文。
	Content string
	// Status 是稳定生命周期状态值。
	Status string
	// TagIDs 是关联标签 ID。
	TagIDs []int64
	// Support 是点赞计数。
	Support int64
	// Comment 是评论计数。
	Comment int64
	// Visited 是访问计数。
	Visited int64
	// Version 是乐观锁版本。
	Version uint64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// ArticleTypeResult 是协议无关的文章分类读取结果。
type ArticleTypeResult struct {
	// ID 是分类 ID。
	ID int64
	// Name 是分类名称。
	Name string
	// Image 是可选分类图片。
	Image *string
	// Meun 是稳定的菜单排序字段名。
	Meun int32
	// Version 是乐观锁版本。
	Version uint64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// TagResult 是协议无关的标签读取结果。
type TagResult struct {
	// ID 是标签 ID。
	ID int64
	// Name 是标签名称。
	Name string
	// Version 是乐观锁版本。
	Version uint64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// ModifiedAt 是最后修改时间。
	ModifiedAt time.Time
}

// ArticlePage 是协议无关的文章分页结果。
type ArticlePage struct {
	// Items 是当前页文章。
	Items []ArticleResult
	// Number 是当前页码。
	Number int
	// Size 是当前页大小。
	Size int
	// TotalItems 是符合条件的总条数。
	TotalItems int64
	// TotalPages 是符合条件的总页数。
	TotalPages int
}

// ArticleTypePage 是协议无关的文章分类分页结果。
type ArticleTypePage struct {
	// Items 是当前页分类。
	Items []ArticleTypeResult
	// Number 是当前页码。
	Number int
	// Size 是当前页大小。
	Size int
	// TotalItems 是符合条件的总条数。
	TotalItems int64
	// TotalPages 是符合条件的总页数。
	TotalPages int
}

// TagPage 是协议无关的标签分页结果。
type TagPage struct {
	// Items 是当前页标签。
	Items []TagResult
	// Number 是当前页码。
	Number int
	// Size 是当前页大小。
	Size int
	// TotalItems 是符合条件的总条数。
	TotalItems int64
	// TotalPages 是符合条件的总页数。
	TotalPages int
}
