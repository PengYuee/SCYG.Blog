package content

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

type cleanupRepositoryFake struct {
	application.ArticleImageRepository
	// images 保存当前图片聚合。
	images map[domain.ArticleImageID]*domain.ArticleImage
	// references 保存锁内引用计数。
	references map[domain.ArticleImageID]int64
	// deleteErr 注入单条元数据删除故障。
	deleteErr map[domain.ArticleImageID]error
}

type cleanupTransactionFake struct {
	application.Transaction
	// repo 是事务内图片仓储。
	repo *cleanupRepositoryFake
}

func (transaction cleanupTransactionFake) ArticleImages() application.ArticleImageRepository {
	return transaction.repo
}

// cleanupUnitFake 同步执行测试事务回调。
type cleanupUnitFake struct {
	// repo 是每次事务使用的仓储。
	repo *cleanupRepositoryFake
}

func (unit cleanupUnitFake) Within(ctx context.Context, callback func(context.Context, application.Transaction) error) error {
	return callback(ctx, cleanupTransactionFake{repo: unit.repo})
}

func (repo *cleanupRepositoryFake) ListExpiredPending(context.Context, time.Time, int) ([]*domain.ArticleImage, error) {
	return repo.byStatus(domain.ArticleImageStatusPending), nil
}

func (repo *cleanupRepositoryFake) ListExpiredOrphaned(context.Context, time.Time, int) ([]*domain.ArticleImage, error) {
	return repo.byStatus(domain.ArticleImageStatusOrphaned), nil
}

func (repo *cleanupRepositoryFake) byStatus(status domain.ArticleImageStatus) []*domain.ArticleImage {
	result := make([]*domain.ArticleImage, 0)
	for _, image := range repo.images {
		if image.Status() == status {
			result = append(result, image)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Metadata().ID.String() < result[right].Metadata().ID.String()
	})
	return result
}

func (repo *cleanupRepositoryFake) FindForUpdate(_ context.Context, ids []domain.ArticleImageID) ([]*domain.ArticleImage, error) {
	if image := repo.images[ids[0]]; image != nil {
		return []*domain.ArticleImage{image}, nil
	}
	return []*domain.ArticleImage{}, nil
}

func (repo *cleanupRepositoryFake) CountReferencesForLockedImage(_ context.Context, id domain.ArticleImageID) (int64, error) {
	return repo.references[id], nil
}

func (repo *cleanupRepositoryFake) DeleteMetadata(_ context.Context, id domain.ArticleImageID) error {
	if err := repo.deleteErr[id]; err != nil {
		return err
	}
	delete(repo.images, id)
	return nil
}

type cleanupStorageFake struct {
	// deleteErr 注入最终文件删除故障。
	deleteErr map[string]error
	// tempErr 注入临时文件删除故障。
	tempErr map[string]error
	// deleted 记录成功删除的名称。
	deleted []string
	// temps 返回待清理临时名称。
	temps []string
}

func (storage *cleanupStorageFake) DeleteArticleImage(key string) error {
	if err := storage.deleteErr[key]; err != nil {
		return err
	}
	storage.deleted = append(storage.deleted, key)
	return nil
}

func (storage *cleanupStorageFake) ListExpiredArticleImageTemps(context.Context, time.Time, int) ([]string, error) {
	return storage.temps, nil
}

func (storage *cleanupStorageFake) DeleteArticleImageTemp(_ context.Context, name string) error {
	if err := storage.tempErr[name]; err != nil {
		return err
	}
	storage.deleted = append(storage.deleted, name)
	return nil
}

// cleanupPendingImage 创建已过期待清理图片。
func cleanupPendingImage(t *testing.T, rawID string, now time.Time) *domain.ArticleImage {
	t.Helper()
	id, err := domain.NewArticleImageID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := domain.NewImageOwnerID("abcdef0123456789abcdef0123456789")
	key, _ := domain.NewStorageKey(rawID + ".jpg")
	image, err := domain.NewArticleImage(domain.ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: domain.MediaTypeJPEG, ByteSize: 1, Width: 1, Height: 1, SHA256: strings.Repeat("a", 64)}, now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return image
}

// cleanupModule 组装清理测试所需的最小模块。
func cleanupModule(now time.Time, repo *cleanupRepositoryFake, storage *cleanupStorageFake) *Module {
	return &Module{clock: imageTestClock{now: now}, unit: cleanupUnitFake{repo: repo}, imageFinalDeleter: storage, imageTempLister: storage, imageTempDeleter: storage, imagePendingTTL: 24 * time.Hour}
}

func Test_CleanupArticleImages_isolates_item_failure_and_retries_next_round(t *testing.T) {
	// Given
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	first := cleanupPendingImage(t, "11111111111111111111111111111111", now)
	second := cleanupPendingImage(t, "22222222222222222222222222222222", now)
	repo := &cleanupRepositoryFake{images: map[domain.ArticleImageID]*domain.ArticleImage{first.Metadata().ID: first, second.Metadata().ID: second}, references: map[domain.ArticleImageID]int64{}, deleteErr: map[domain.ArticleImageID]error{}}
	storage := &cleanupStorageFake{deleteErr: map[string]error{first.Metadata().StorageKey.String(): errors.New("文件被占用")}}
	module := cleanupModule(now, repo, storage)

	// When
	firstErr := module.CleanupArticleImages(context.Background())
	failedRemains := repo.images[first.Metadata().ID] != nil
	succeededRemoved := repo.images[second.Metadata().ID] == nil
	firstRoundDeleted := append([]string(nil), storage.deleted...)
	delete(storage.deleteErr, first.Metadata().StorageKey.String())
	secondErr := module.CleanupArticleImages(context.Background())

	// Then
	if firstErr == nil || !failedRemains || !succeededRemoved || len(firstRoundDeleted) != 1 || firstRoundDeleted[0] != second.Metadata().StorageKey.String() || secondErr != nil || len(repo.images) != 0 {
		t.Fatalf("firstErr=%v failedRemains=%t succeededRemoved=%t firstRoundDeleted=%v secondErr=%v remaining=%d", firstErr, failedRemains, succeededRemoved, firstRoundDeleted, secondErr, len(repo.images))
	}
}

func Test_CleanupArticleImages_retries_metadata_failure_after_file_is_missing(t *testing.T) {
	// Given
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	image := cleanupPendingImage(t, "44444444444444444444444444444444", now)
	repo := &cleanupRepositoryFake{images: map[domain.ArticleImageID]*domain.ArticleImage{image.Metadata().ID: image}, references: map[domain.ArticleImageID]int64{}, deleteErr: map[domain.ArticleImageID]error{image.Metadata().ID: errors.New("数据库删除失败")}}
	storage := &cleanupStorageFake{deleteErr: map[string]error{}}
	module := cleanupModule(now, repo, storage)

	// When
	firstErr := module.CleanupArticleImages(context.Background())
	delete(repo.deleteErr, image.Metadata().ID)
	secondErr := module.CleanupArticleImages(context.Background())

	// Then
	if firstErr == nil || secondErr != nil || len(repo.images) != 0 || len(storage.deleted) != 2 {
		t.Fatalf("firstErr=%v secondErr=%v remaining=%d deleted=%v", firstErr, secondErr, len(repo.images), storage.deleted)
	}
}

func Test_CleanupArticleImages_isolates_temp_failure(t *testing.T) {
	// Given
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	repo := &cleanupRepositoryFake{images: map[domain.ArticleImageID]*domain.ArticleImage{}, references: map[domain.ArticleImageID]int64{}, deleteErr: map[domain.ArticleImageID]error{}}
	storage := &cleanupStorageFake{deleteErr: map[string]error{}, tempErr: map[string]error{"first.tmp": errors.New("临时文件被占用")}, temps: []string{"first.tmp", "second.tmp"}}
	module := cleanupModule(now, repo, storage)

	// When
	err := module.CleanupArticleImages(context.Background())

	// Then
	if err == nil || len(storage.deleted) != 1 || storage.deleted[0] != "second.tmp" {
		t.Fatalf("err=%v deleted=%v", err, storage.deleted)
	}
}

func Test_CleanupArticleImages_deletes_only_unreferenced_orphan(t *testing.T) {
	// Given
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	unreferenced := cleanupPendingImage(t, "55555555555555555555555555555555", now.Add(-24*time.Hour))
	referenced := cleanupPendingImage(t, "66666666666666666666666666666666", now.Add(-24*time.Hour))
	for _, image := range []*domain.ArticleImage{unreferenced, referenced} {
		if err := image.Commit(now.Add(-60 * time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := image.Orphan(now.Add(-25 * time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	repo := &cleanupRepositoryFake{images: map[domain.ArticleImageID]*domain.ArticleImage{unreferenced.Metadata().ID: unreferenced, referenced.Metadata().ID: referenced}, references: map[domain.ArticleImageID]int64{referenced.Metadata().ID: 1}, deleteErr: map[domain.ArticleImageID]error{}}
	storage := &cleanupStorageFake{deleteErr: map[string]error{}}
	module := cleanupModule(now, repo, storage)

	// When
	err := module.CleanupArticleImages(context.Background())

	// Then
	if err != nil || len(repo.images) != 1 || repo.images[referenced.Metadata().ID] == nil {
		t.Fatalf("err=%v remaining=%v", err, repo.images)
	}
}
