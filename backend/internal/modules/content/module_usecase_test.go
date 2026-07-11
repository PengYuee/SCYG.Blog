package content_test

import (
	"context"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type countingUnitOfWork struct{ calls int }

func (unit *countingUnitOfWork) Within(context.Context, func(context.Context, application.Transaction) error) error {
	unit.calls++
	return nil
}

type allowAll struct{ sequence *[]string }

func (authorizer allowAll) Authorize(context.Context, content.Action, content.Resource) error {
	*authorizer.sequence = append(*authorizer.sequence, "authorize")
	return nil
}

type recordingUnit struct {
	sequence    *[]string
	transaction application.Transaction
}

func (unit recordingUnit) Within(ctx context.Context, callback func(context.Context, application.Transaction) error) error {
	*unit.sequence = append(*unit.sequence, "uow")
	return callback(ctx, unit.transaction)
}

type recordingTransaction struct{ articles *recordingArticles }

func (transaction recordingTransaction) Articles() application.ArticleRepository {
	return transaction.articles
}
func (recordingTransaction) ArticleTypes() application.ArticleTypeRepository {
	return unusedArticleTypes{}
}
func (recordingTransaction) Tags() application.TagRepository { return unusedTags{} }

type recordingArticles struct{ sequence *[]string }

func (repo *recordingArticles) NextID(context.Context) (domain.ArticleID, error) {
	*repo.sequence = append(*repo.sequence, "next-id")
	return domain.NewArticleID(7)
}
func (*recordingArticles) Find(context.Context, domain.ArticleID) (*domain.Article, error) {
	return nil, content.ErrNotFound
}
func (repo *recordingArticles) Save(context.Context, *domain.Article) error {
	*repo.sequence = append(*repo.sequence, "save")
	return nil
}

type unusedArticleTypes struct{}

func (unusedArticleTypes) NextID(context.Context) (domain.ArticleTypeID, error) {
	return domain.NewArticleTypeID(1)
}
func (unusedArticleTypes) Find(context.Context, domain.ArticleTypeID) (*domain.ArticleType, error) {
	return nil, content.ErrNotFound
}
func (unusedArticleTypes) Save(context.Context, *domain.ArticleType) error { return nil }

type unusedTags struct{}

func (unusedTags) NextID(context.Context) (domain.TagID, error) { return domain.NewTagID(1) }
func (unusedTags) Find(context.Context, domain.TagID) (*domain.Tag, error) {
	return nil, content.ErrNotFound
}
func (unusedTags) Save(context.Context, *domain.Tag) error { return nil }

type unusedReadModel struct{}

func (unusedReadModel) FindPublished(context.Context, domain.ArticleID) (application.ArticleView, error) {
	return application.ArticleView{}, nil
}
func (unusedReadModel) ListPublished(context.Context, application.ArticleFilter) (application.ArticlePage, error) {
	return application.ArticlePage{}, nil
}
func (unusedReadModel) FindArticleType(context.Context, domain.ArticleTypeID) (application.ArticleTypeView, error) {
	return application.ArticleTypeView{}, nil
}
func (unusedReadModel) ListArticleTypes(context.Context, string) ([]application.ArticleTypeView, error) {
	return nil, nil
}
func (unusedReadModel) FindTag(context.Context, domain.TagID) (application.TagView, error) {
	return application.TagView{}, nil
}
func (unusedReadModel) ListTags(context.Context, string) ([]application.TagView, error) {
	return nil, nil
}

func Test_ContentUseCase_DenyAll_stops_before_unit_of_work(t *testing.T) {
	// Given
	unit := &countingUnitOfWork{}
	module, err := content.NewModule(content.Dependencies{Clock: fixedClock{now: time.Unix(1, 0)}, UnitOfWork: unit, Articles: unusedReadModel{}, Taxonomies: unusedReadModel{}})
	if err != nil {
		t.Fatalf("NewModule() error = %v", err)
	}

	// When
	_, commandErr := module.CreateArticle(context.Background(), content.CreateArticle{ArticleTypeID: 1, Title: "Title", Slug: "title", Digest: "Digest", Content: "Body", TagIDs: []int64{1}})

	// Then
	if commandErr == nil {
		t.Fatal("CreateArticle() error = nil")
	}
	if unit.calls != 0 {
		t.Fatalf("UnitOfWork.Within() calls = %d, want 0", unit.calls)
	}
}

func Test_ContentUseCase_CreateArticle_calls_authorization_before_transaction_ports(t *testing.T) {
	// Given
	sequence := []string{}
	articles := &recordingArticles{sequence: &sequence}
	unit := recordingUnit{sequence: &sequence, transaction: recordingTransaction{articles: articles}}
	module, err := content.NewModule(content.Dependencies{Clock: fixedClock{now: time.Unix(1, 0)}, Authorizer: allowAll{sequence: &sequence}, UnitOfWork: unit, Articles: unusedReadModel{}, Taxonomies: unusedReadModel{}})
	if err != nil {
		t.Fatalf("NewModule() error = %v", err)
	}

	// When
	result, commandErr := module.CreateArticle(context.Background(), content.CreateArticle{ArticleTypeID: 1, Title: "Title", Slug: "title", Digest: "Digest", Content: "Body", TagIDs: []int64{1}})

	// Then
	if commandErr != nil {
		t.Fatalf("CreateArticle() error = %v", commandErr)
	}
	want := []string{"authorize", "uow", "next-id", "save"}
	if len(sequence) != len(want) {
		t.Fatalf("sequence = %v", sequence)
	}
	for index := range want {
		if sequence[index] != want[index] {
			t.Fatalf("sequence = %v, want %v", sequence, want)
		}
	}
	if result.ID != 7 || result.Version != 1 {
		t.Fatalf("result = %#v", result)
	}
}
