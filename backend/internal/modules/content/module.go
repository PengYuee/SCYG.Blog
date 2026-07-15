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
	// ArticleImagePolicy 是图片上传、读取与生命周期共享的不可变策略。
	ArticleImagePolicy ArticleImagePolicy
	// ArticleImageFinalDeleter 提供后台最终文件删除能力。
	ArticleImageFinalDeleter ArticleImageFinalDeleter
	// ArticleImageTempLister 提供后台过期临时文件枚举能力。
	ArticleImageTempLister ArticleImageTempLister
	// ArticleImageTempDeleter 提供后台临时文件删除能力。
	ArticleImageTempDeleter ArticleImageTempDeleter
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
	// imagePolicy 是上传校验与 orphan 生命周期共享的不可变策略。
	imagePolicy ArticleImagePolicy
	// imageFinalDeleter 执行最终文件幂等删除。
	imageFinalDeleter ArticleImageFinalDeleter
	// imageTempLister 枚举过期临时文件。
	imageTempLister ArticleImageTempLister
	// imageTempDeleter 执行临时文件幂等删除。
	imageTempDeleter ArticleImageTempDeleter
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
	policy := dependencies.ArticleImagePolicy.orDefault()
	return &Module{clock: dependencies.Clock, authorizer: AuthorizerOrDeny(dependencies.Authorizer), currentAuthor: CurrentAuthorProviderOrUnavailable(dependencies.CurrentAuthor), unit: dependencies.UnitOfWork, articles: dependencies.Articles, taxonomies: dependencies.Taxonomies, imageStager: articleImageStagerOrUnavailable(dependencies.ArticleImageStager), imagePublisher: articleImagePublisherOrUnavailable(dependencies.ArticleImagePublisher), imageDiscarder: articleImageDiscarderOrUnavailable(dependencies.ArticleImageDiscarder), imageLoader: articleImageLoaderOrUnavailable(dependencies.ArticleImageLoader), imagePendingTTL: policy.PendingTTL(), imagePolicy: policy, imageFinalDeleter: articleImageFinalDeleterOrUnavailable(dependencies.ArticleImageFinalDeleter), imageTempLister: articleImageTempListerOrUnavailable(dependencies.ArticleImageTempLister), imageTempDeleter: articleImageTempDeleterOrUnavailable(dependencies.ArticleImageTempDeleter)}, nil
}

// ArticleImagePolicy 返回 REST 与存储必须共享的不可变图片策略。
func (module *Module) ArticleImagePolicy() ArticleImagePolicy {
	return module.imagePolicy.orDefault()
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
