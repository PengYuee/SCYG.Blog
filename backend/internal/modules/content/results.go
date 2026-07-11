package content

import (
	"errors"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func articleResult(article *domain.Article) ArticleResult {
	tags := article.TagIDs()
	tagIDs := make([]int64, len(tags))
	for index, id := range tags {
		tagIDs[index] = id.Int64()
	}
	return ArticleResult{ID: article.ID().Int64(), ArticleTypeID: article.ArticleTypeID().Int64(), Title: article.Title().String(), Slug: article.Slug().String(), Digest: article.Digest().String(), Content: article.Content().String(), Status: string(article.Status()), TagIDs: tagIDs, Version: article.Version().Uint64(), CreatedAt: article.CreatedAt(), ModifiedAt: article.ModifiedAt()}
}

func articleViewResult(view application.ArticleView) ArticleResult {
	tagIDs := make([]int64, len(view.TagIDs))
	for index, id := range view.TagIDs {
		tagIDs[index] = id.Int64()
	}
	return ArticleResult{ID: view.ID.Int64(), ArticleTypeID: view.ArticleTypeID.Int64(), Title: view.Title.String(), Slug: view.Slug.String(), Digest: view.Digest.String(), Content: view.Content.String(), Status: string(view.Status), TagIDs: tagIDs, Version: view.Version.Uint64(), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt}
}

func articleTypeResult(item *domain.ArticleType) ArticleTypeResult {
	return ArticleTypeResult{ID: item.ID().Int64(), Name: item.Name().String(), Version: item.Version().Uint64(), CreatedAt: item.CreatedAt(), ModifiedAt: item.ModifiedAt()}
}
func tagResult(item *domain.Tag) TagResult {
	return TagResult{ID: item.ID().Int64(), Name: item.Name().String(), Version: item.Version().Uint64(), CreatedAt: item.CreatedAt(), ModifiedAt: item.ModifiedAt()}
}
func articleTypeViewResult(view application.ArticleTypeView) ArticleTypeResult {
	return ArticleTypeResult{ID: view.ID.Int64(), Name: view.Name.String(), Version: view.Version.Uint64(), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt}
}
func tagViewResult(view application.TagView) TagResult {
	return TagResult{ID: view.ID.Int64(), Name: view.Name.String(), Version: view.Version.Uint64(), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt}
}

func validation(err error) error {
	return &ApplicationError{Code: CodeValidation, Kind: KindValidation, Cause: err}
}
func permission(err error) error {
	if errors.Is(err, ErrPermissionDenied) {
		return err
	}
	return &ApplicationError{Code: CodePermissionDenied, Kind: KindPermission, Cause: ErrPermissionDenied}
}
func stable(err error) error {
	if err == nil {
		return nil
	}
	var known *ApplicationError
	if errors.As(err, &known) {
		return known
	}
	var conflict *domain.VersionConflict
	if errors.As(err, &conflict) {
		return &ApplicationError{Code: CodeStaleVersion, Kind: KindConflict, Cause: domain.ErrStaleVersion, ExpectedVersion: conflict.Expected.Uint64(), ActualVersion: conflict.Actual.Uint64()}
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return &ApplicationError{Code: CodeNotFound, Kind: KindMissing, Cause: ErrNotFound}
	case errors.Is(err, ErrConflict):
		return &ApplicationError{Code: CodeAlreadyExists, Kind: KindConflict, Cause: ErrConflict}
	case errors.Is(err, ErrFailedPrecondition), errors.Is(err, domain.ErrNoChange), errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrDeleted):
		return &ApplicationError{Code: CodeFailedPrecondition, Kind: KindConflict, Cause: ErrFailedPrecondition}
	case errors.Is(err, domain.ErrInvalidValue), errors.Is(err, domain.ErrDuplicateTag):
		return validation(err)
	default:
		return &ApplicationError{Code: CodeInternal, Kind: KindInternal, Cause: ErrPersistence}
	}
}
