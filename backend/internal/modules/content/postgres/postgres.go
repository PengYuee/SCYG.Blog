// Package postgres 在内容模块边界内组合私有 PostgreSQL 与图片存储适配器。
package postgres

import (
	"errors"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	contentpostgres "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/postgres"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

// Dependencies 是内容模块 PostgreSQL 组合所需的显式依赖。
type Dependencies struct {
	// Database 是已连通且由应用生命周期持有的数据库。
	Database *database.Database
	// Authorizer 是可替换授权策略；nil 时由模块安全降级为 DenyAll。
	Authorizer module.Authorizer
	// CurrentAuthor 是可替换可信作者来源；nil 时由模块安全降级为不可用。
	CurrentAuthor module.CurrentAuthorProvider
	// ImageFilesystem 是由 bootstrap 持有生命周期的固定根存储。
	ImageFilesystem *blobstorage.Filesystem
	// ImagePolicy 是图片安全与生命周期共享的不可变策略。
	ImagePolicy module.ArticleImagePolicy
}

// New 构造私有持久化适配器并调用 content.NewModule。
func New(dependencies Dependencies) (*module.Module, error) {
	if dependencies.Database == nil {
		return nil, errors.New("内容数据库为空")
	}
	transaction, err := database.NewUnitOfWork(dependencies.Database)
	if err != nil {
		return nil, err
	}
	unit, err := contentpostgres.NewUnitOfWork(transaction)
	if err != nil {
		return nil, err
	}
	read, err := contentpostgres.NewReadModel(dependencies.Database.GORM())
	if err != nil {
		return nil, err
	}
	var storage *ImageStorage
	if dependencies.ImageFilesystem != nil {
		storage = NewImageStorage(dependencies.ImageFilesystem, dependencies.ImagePolicy)
	}
	return module.NewModule(module.Dependencies{Clock: systemClock{}, Authorizer: dependencies.Authorizer, CurrentAuthor: dependencies.CurrentAuthor, UnitOfWork: unit, Articles: read, Taxonomies: read, ArticleImageStager: storage, ArticleImagePublisher: storage, ArticleImageDiscarder: storage, ArticleImageLoader: storage, ArticleImagePolicy: dependencies.ImagePolicy, ArticleImageFinalDeleter: storage, ArticleImageTempLister: storage, ArticleImageTempDeleter: storage})
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
