package content

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

type imageTestClock struct{ now time.Time }

func (clock imageTestClock) Now() time.Time { return clock.now }

type imageAllowAll struct{}

func (imageAllowAll) Authorize(context.Context, Action, Resource) error { return nil }

type imageTestContent struct{ reader *bytes.Reader }

func (content *imageTestContent) ReadArticleImage(buffer []byte) (int, error) {
	return content.reader.Read(buffer)
}

type imageTestRepo struct {
	application.ArticleImageRepository
	saved     *domain.ArticleImage
	saveErr   error
	deleted   int
	deleteErr error
}

func (repo *imageTestRepo) Save(_ context.Context, image *domain.ArticleImage) error {
	repo.saved = image
	if repo.saveErr != nil {
		return repo.saveErr
	}
	return nil
}

func (repo *imageTestRepo) DeleteMetadata(context.Context, domain.ArticleImageID) error {
	repo.deleted++
	return repo.deleteErr
}

type imageTestTransaction struct {
	application.Transaction
	repo *imageTestRepo
}

func (transaction imageTestTransaction) ArticleImages() application.ArticleImageRepository {
	return transaction.repo
}

type imageTestUnit struct{ repo *imageTestRepo }

func (unit imageTestUnit) Within(ctx context.Context, callback func(context.Context, application.Transaction) error) error {
	return callback(ctx, imageTestTransaction{repo: unit.repo})
}

type imageCommittedError struct{}

func (imageCommittedError) Error() string   { return "已提交" }
func (imageCommittedError) Committed() bool { return true }

type imageTestAssets struct {
	staged    []byte
	discarded int
	commitErr error
}

func (assets *imageTestAssets) StageArticleImage(_ context.Context, _ string, content ArticleImageContent) (string, int64, error) {
	buffer := make([]byte, 1024)
	for {
		count, err := content.ReadArticleImage(buffer)
		assets.staged = append(assets.staged, buffer[:count]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", 0, err
		}
	}
	return ".article-image-token", int64(len(assets.staged)), nil
}
func (assets *imageTestAssets) CommitArticleImage(string, string) error { return assets.commitErr }
func (assets *imageTestAssets) DiscardArticleImage(context.Context, string) error {
	assets.discarded++
	return nil
}

func jpegContent(t *testing.T) *imageTestContent {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, nil); err != nil {
		t.Fatal(err)
	}
	return &imageTestContent{reader: bytes.NewReader(encoded.Bytes())}
}

func imageModuleForTest(t *testing.T, repo *imageTestRepo, assets *imageTestAssets) *Module {
	t.Helper()
	author, _ := NewAuthorID("abcdef0123456789abcdef0123456789")
	return &Module{clock: imageTestClock{now: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)}, authorizer: imageAllowAll{}, currentAuthor: NewFixedCurrentAuthorProvider(author), unit: imageTestUnit{repo: repo}, imageStager: assets, imagePublisher: assets, imageDiscarder: assets, imagePendingTTL: 24 * time.Hour}
}

func Test_UploadArticleImage_keeps_pending_metadata_when_commit_reports_committed(t *testing.T) {
	repo := &imageTestRepo{}
	assets := &imageTestAssets{commitErr: imageCommittedError{}}
	result, err := imageModuleForTest(t, repo, assets).UploadArticleImage(context.Background(), UploadArticleImage{Content: jpegContent(t)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending" || repo.deleted != 0 || assets.discarded != 1 {
		t.Fatalf("result=%+v deleted=%d discarded=%d", result, repo.deleted, assets.discarded)
	}
}

func Test_UploadArticleImage_discards_temp_without_metadata_when_save_fails(t *testing.T) {
	repo := &imageTestRepo{saveErr: errors.New("database")}
	assets := &imageTestAssets{}
	_, err := imageModuleForTest(t, repo, assets).UploadArticleImage(context.Background(), UploadArticleImage{Content: jpegContent(t)})
	if err == nil || assets.discarded != 1 {
		t.Fatalf("error=%v discarded=%d", err, assets.discarded)
	}
}

func Test_UploadArticleImage_compensates_metadata_and_temp_when_commit_has_no_final(t *testing.T) {
	repo := &imageTestRepo{}
	assets := &imageTestAssets{commitErr: errors.New("link")}
	_, err := imageModuleForTest(t, repo, assets).UploadArticleImage(context.Background(), UploadArticleImage{Content: jpegContent(t)})
	if err == nil || repo.deleted != 1 || assets.discarded != 1 {
		t.Fatalf("error=%v deleted=%d discarded=%d", err, repo.deleted, assets.discarded)
	}
}

func Test_UploadArticleImage_preserves_pending_metadata_when_compensation_fails(t *testing.T) {
	repo := &imageTestRepo{deleteErr: errors.New("delete metadata")}
	assets := &imageTestAssets{commitErr: errors.New("link")}
	_, err := imageModuleForTest(t, repo, assets).UploadArticleImage(context.Background(), UploadArticleImage{Content: jpegContent(t)})
	if err == nil || repo.saved == nil || repo.deleted != 1 {
		t.Fatalf("error=%v saved=%v deleted=%d", err, repo.saved, repo.deleted)
	}
}
