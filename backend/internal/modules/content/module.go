package content

import (
	"errors"
	"reflect"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
)

// Clock 为用例提供确定性时间。
type (
	Clock = application.Clock
	// UnitOfWork 管理事务范围内的内容仓储，写操作必须通过它保持原子性。
	UnitOfWork = application.UnitOfWork
	// ArticleReadModel 提供已发布文章投影。
	ArticleReadModel = application.ArticleReadModel
	// TaxonomyReadModel 提供未删除分类和标签投影。
	TaxonomyReadModel = application.TaxonomyReadModel
)

// Dependencies 包含构造 Module 所需的显式协议无关协作者。
type Dependencies struct {
	// Clock 是用例使用的领域时钟。
	Clock Clock
	// Authorizer 是可选鉴权器，缺省时安全降级为 DenyAll。
	Authorizer Authorizer
	// CurrentAuthor 是可选可信作者来源，缺省时安全降级为不可用。
	CurrentAuthor CurrentAuthorProvider
	// UnitOfWork 是所有写操作的事务边界。
	UnitOfWork UnitOfWork
	// Articles 提供公开文章读取投影。
	Articles ArticleReadModel
	// Taxonomies 提供分类与标签读取投影。
	Taxonomies TaxonomyReadModel
	// ArticleImageStager 提供验证后图片暂存。
	ArticleImageStager ArticleImageStager
	// ArticleImageCommitter 提供图片提交与暂存丢弃。
	ArticleImagePublisher ArticleImagePublisher
	// ArticleImageDiscarder 提供暂存丢弃。
	ArticleImageDiscarder ArticleImageDiscarder
	// ArticleImageLoader 提供最终图片有界读取。
	ArticleImageLoader ArticleImageLoader
	// ArticleImagePendingTTL 是上传图片等待文章确认的期限；零值使用 24 小时。
	ArticleImagePendingTTL time.Duration
}

// Module 是具体的协议无关内容门面。
type Module struct {
	clock           Clock
	authorizer      Authorizer
	currentAuthor   CurrentAuthorProvider
	unit            UnitOfWork
	articles        ArticleReadModel
	taxonomies      TaxonomyReadModel
	imageStager     ArticleImageStager
	imagePublisher  ArticleImagePublisher
	imageDiscarder  ArticleImageDiscarder
	imageLoader     ArticleImageLoader
	imagePendingTTL time.Duration
}

// NewModule 安全组装全部内容用例；可选依赖均按最小权限安全降级。
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
	ttl := dependencies.ArticleImagePendingTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Module{clock: dependencies.Clock, authorizer: AuthorizerOrDeny(dependencies.Authorizer), currentAuthor: CurrentAuthorProviderOrUnavailable(dependencies.CurrentAuthor), unit: dependencies.UnitOfWork, articles: dependencies.Articles, taxonomies: dependencies.Taxonomies, imageStager: articleImageStagerOrUnavailable(dependencies.ArticleImageStager), imagePublisher: articleImagePublisherOrUnavailable(dependencies.ArticleImagePublisher), imageDiscarder: articleImageDiscarderOrUnavailable(dependencies.ArticleImageDiscarder), imageLoader: articleImageLoaderOrUnavailable(dependencies.ArticleImageLoader), imagePendingTTL: ttl}, nil
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
