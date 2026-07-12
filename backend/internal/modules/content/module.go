package content

import (
	"errors"
	"reflect"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
)

// Clock 为用例提供确定性时间。
type Clock = application.Clock

// UnitOfWork 管理事务范围内的内容仓储，写操作必须通过它保持原子性。
type UnitOfWork = application.UnitOfWork

// ArticleReadModel 提供已发布文章投影。
type ArticleReadModel = application.ArticleReadModel

// TaxonomyReadModel 提供未删除分类和标签投影。
type TaxonomyReadModel = application.TaxonomyReadModel

// Dependencies 包含构造 Module 所需的显式协议无关协作者。
type Dependencies struct {
	// Clock 是用例使用的领域时钟。
	Clock Clock
	// Authorizer 是可选鉴权器，缺省时安全降级为 DenyAll。
	Authorizer Authorizer
	// UnitOfWork 是所有写操作的事务边界。
	UnitOfWork UnitOfWork
	// Articles 提供公开文章读取投影。
	Articles ArticleReadModel
	// Taxonomies 提供分类与标签读取投影。
	Taxonomies TaxonomyReadModel
}

// Module 是具体的协议无关内容门面。
type Module struct {
	clock      Clock
	authorizer Authorizer
	unit       UnitOfWork
	articles   ArticleReadModel
	taxonomies TaxonomyReadModel
}

// NewModule 安全组装全部内容用例；省略鉴权器时默认 DenyAll，其他依赖不得为 nil。
func NewModule(dependencies Dependencies) (*Module, error) {
	if nilLike(dependencies.Clock) {
		return nil, errors.New("内容时钟为空")
	}
	if nilLike(dependencies.UnitOfWork) {
		return nil, errors.New("内容工作单元为空")
	}
	if nilLike(dependencies.Articles) {
		return nil, errors.New("内容文章读模型为空")
	}
	if nilLike(dependencies.Taxonomies) {
		return nil, errors.New("内容分类读模型为空")
	}
	return &Module{clock: dependencies.Clock, authorizer: AuthorizerOrDeny(dependencies.Authorizer), unit: dependencies.UnitOfWork, articles: dependencies.Articles, taxonomies: dependencies.Taxonomies}, nil
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
