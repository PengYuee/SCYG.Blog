package content

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func Test_ArticleImagePolicy_defaults_preserve_existing_runtime_behavior(t *testing.T) {
	// When
	policy := DefaultArticleImagePolicy()

	// Then
	if policy.MaxFileBytes() != 5<<20 || policy.MaxPixels() != 25_000_000 || policy.MaxDimension() != 8192 || policy.PendingTTL() != 24*time.Hour || policy.OrphanGrace() != 24*time.Hour {
		t.Fatalf("默认图片策略不一致：%+v", policy)
	}
}

func Test_UploadArticleImage_uses_custom_validation_limits(t *testing.T) {
	// Given
	repo := &imageTestRepo{}
	assets := &imageTestAssets{}
	module := imageModuleForTest(t, repo, assets)
	module.imagePolicy = NewArticleImagePolicy(ArticleImagePolicyOptions{MaxFileBytes: 1, MaxPixels: 1, MaxDimension: 1, PendingTTL: time.Hour, OrphanGrace: time.Hour})

	// When
	_, err := module.UploadArticleImage(context.Background(), UploadArticleImage{Content: jpegContent(t)})

	// Then
	var applicationErr *ApplicationError
	if !errors.As(err, &applicationErr) || !errors.Is(applicationErr.Cause, domain.ErrArticleImageTooLarge) {
		t.Fatalf("期望自定义文件上限错误，实际 %v", err)
	}
	if len(assets.staged) != 0 {
		t.Fatalf("被拒图片不应进入暂存：%d", len(assets.staged))
	}
}

type policyReferenceRepository struct {
	application.ArticleImageRepository
	image *domain.ArticleImage
	saved *domain.ArticleImage
}

func (repository *policyReferenceRepository) FindArticleReferences(context.Context, domain.ArticleID) ([]domain.ArticleImageID, error) {
	return []domain.ArticleImageID{repository.image.Metadata().ID}, nil
}

func (repository *policyReferenceRepository) LockReferenceTransition(context.Context, []domain.ArticleImageID, []domain.StorageKey) ([]*domain.ArticleImage, error) {
	return []*domain.ArticleImage{repository.image}, nil
}

func (*policyReferenceRepository) ReplaceArticleReferences(context.Context, domain.ArticleID, []domain.ArticleImageID, time.Time) error {
	return nil
}

func (*policyReferenceRepository) CountReferencesForLockedImage(context.Context, domain.ArticleImageID) (int64, error) {
	return 0, nil
}

func (repository *policyReferenceRepository) Save(_ context.Context, image *domain.ArticleImage) error {
	repository.saved = image
	return nil
}

func Test_bindArticleImages_last_reference_uses_custom_orphan_grace(t *testing.T) {
	// Given
	const grace = 30 * time.Minute
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	id, _ := domain.NewArticleImageID("abcdef0123456789abcdef0123456789")
	owner, _ := domain.NewImageOwnerID("0123456789abcdef0123456789abcdef")
	key, _ := domain.NewStorageKey("abcdef0123456789abcdef0123456789.jpg")
	image, err := domain.NewArticleImage(domain.ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: domain.MediaTypeJPEG, ByteSize: 1, Width: 1, Height: 1, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, now.Add(-2*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = image.Commit(now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	articleID, _ := domain.NewArticleID(1)
	repository := &policyReferenceRepository{image: image}
	module := &Module{imagePolicy: NewArticleImagePolicy(ArticleImagePolicyOptions{MaxFileBytes: 1, MaxPixels: 1, MaxDimension: 1, PendingTTL: time.Hour, OrphanGrace: grace})}

	// When
	err = module.bindArticleImages(context.Background(), repository, articleID, nil, articleImageIdentity{}, now)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if repository.saved == nil || repository.saved.Status() != domain.ArticleImageStatusOrphaned || !repository.saved.ExpiresAt().Equal(now.Add(grace)) {
		t.Fatalf("最后引用移除未使用自定义宽限期：saved=%v expires=%v", repository.saved, image.ExpiresAt())
	}
}
