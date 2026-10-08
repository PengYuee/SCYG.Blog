package article

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// Service executes article commands and delegates persistence to the feature repository.
type Service struct {
	db         *gorm.DB
	repo       *Repository
	query      *Query
	clock      Clock
	authorizer content.Authorizer
}

// New constructs an article service with its query and repository collaborators.
func New(db *gorm.DB, dependencies Dependencies) (*Service, error) {
	if db == nil {
		return nil, errors.New("article database is nil")
	}
	if dependencies.Clock == nil {
		return nil, errors.New("article clock is nil")
	}
	repo, err := NewRepository(db)
	if err != nil {
		return nil, err
	}
	query, err := NewQuery(db)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, repo: repo, query: query, clock: dependencies.Clock, authorizer: content.AuthorizerOrDeny(dependencies.Authorizer)}, nil
}

// NewService is the named constructor retained for feature composition.
func NewService(db *gorm.DB, dependencies Dependencies) (*Service, error) {
	return New(db, dependencies)
}

// ValidateCreate parses creation status and article fields without database access.
func (service *Service) ValidateCreate(input Create) (Status, error) {
	status, err := parseCreationStatus(input.Status)
	if err != nil {
		return "", validation(err)
	}
	if _, err := parseCreate(input); err != nil {
		return "", validation(err)
	}
	return status, nil
}

// ValidatePatch parses the identity before authorization.
func (service *Service) ValidatePatch(input Patch) error {
	if _, _, err := parseIdentity(input.ID, input.Version); err != nil {
		return validation(err)
	}
	if input.ArticleTypeID == nil && input.Title == nil && input.Slug == nil && input.Digest == nil && input.Content == nil && input.TagIDs == nil {
		return validation(ErrInvalidValue)
	}
	return nil
}

// Create authorizes and persists a new article.
func (service *Service) Create(ctx context.Context, input Create) (Result, error) {
	status, err := service.ValidateCreate(input)
	if err != nil {
		return Result{}, err
	}
	if err := authorize(service.authorizer, ctx, ActionCreateArticle, "article", 0); err != nil {
		return Result{}, err
	}
	if status == StatusPublished {
		if err := authorize(service.authorizer, ctx, ActionPublishArticle, "article", 0); err != nil {
			return Result{}, err
		}
	}
	var result Result
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		value, err := service.CreateInTx(ctx, tx, input)
		if err == nil {
			result = value
		}
		return err
	})
	return result, stable(err)
}

// CreateInTx creates an article using the caller-owned transaction.
func (service *Service) CreateInTx(ctx context.Context, db *gorm.DB, input Create) (Result, error) {
	if db == nil {
		return Result{}, internal(errors.New("article transaction is nil"))
	}
	status, err := parseCreationStatus(input.Status)
	if err != nil {
		return Result{}, validation(err)
	}
	draft, err := parseCreate(input)
	if err != nil {
		return Result{}, validation(err)
	}
	repo := &Repository{db: db}
	id, err := repo.NextID(ctx)
	if err != nil {
		return Result{}, err
	}
	draft.ID = id
	value, err := NewArticle(draft, status, service.clock)
	if err != nil {
		return Result{}, err
	}
	if err := repo.Save(ctx, value); err != nil {
		return Result{}, err
	}
	return result(value), nil
}

// PreviewPatch computes a version-checked patch before image preparation and the write transaction.
func (service *Service) PreviewPatch(ctx context.Context, input Patch) (PatchPreview, error) {
	id, version, err := parseIdentity(input.ID, input.Version)
	if err != nil {
		return PatchPreview{}, validation(err)
	}
	repo := service.repo
	value, err := repo.Find(ctx, id)
	if err != nil {
		return PatchPreview{}, err
	}
	if err := applyPatch(value, version, input, service.clock); err != nil {
		return PatchPreview{}, err
	}
	keys, err := ManagedImageReferences(value.Content().String())
	if err != nil {
		return PatchPreview{}, validation(err)
	}
	return PatchPreview{Article: value, Keys: keys}, nil
}

// Revise replaces editable article fields after version and authorization checks.
func (service *Service) Revise(ctx context.Context, input Revise) (Result, error) {
	id, _, err := parseIdentity(input.ID, input.Version)
	if err != nil {
		return Result{}, validation(err)
	}
	if err := authorize(service.authorizer, ctx, ActionReviseArticle, "article", id.Int64()); err != nil {
		return Result{}, err
	}
	var out Result
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		value, err := service.reviseInTx(ctx, tx, input)
		if err == nil {
			out = value
		}
		return err
	})
	return out, stable(err)
}

func (service *Service) reviseInTx(ctx context.Context, db *gorm.DB, input Revise) (Result, error) {
	id, version, err := parseIdentity(input.ID, input.Version)
	if err != nil {
		return Result{}, validation(err)
	}
	draft, err := parseCreate(Create{ArticleTypeID: input.ArticleTypeID, Title: input.Title, Slug: input.Slug, Digest: input.Digest, Content: input.Content, TagIDs: input.TagIDs})
	if err != nil {
		return Result{}, validation(err)
	}
	repo := &Repository{db: db}
	value, err := repo.Find(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if err := value.Revise(version, ArticleRevision{ArticleTypeID: draft.ArticleTypeID, Title: draft.Title, Slug: draft.Slug, Digest: draft.Digest, Content: draft.Content, TagIDs: draft.TagIDs}, service.clock); err != nil {
		return Result{}, err
	}
	if err := repo.Save(ctx, value); err != nil {
		return Result{}, err
	}
	return result(value), nil
}

// Publish transitions an article to the published state.
func (service *Service) Publish(ctx context.Context, input Publish) (Result, error) {
	return service.transition(ctx, input.ID, input.Version, ActionPublishArticle, func(value *Article, version Version) error { return value.Publish(version, service.clock) })
}

// Archive transitions an article to the archived state.
func (service *Service) Archive(ctx context.Context, input Archive) (Result, error) {
	return service.transition(ctx, input.ID, input.Version, ActionArchiveArticle, func(value *Article, version Version) error { return value.Archive(version, service.clock) })
}

func (service *Service) transition(ctx context.Context, idRaw int64, versionRaw uint64, action content.Action, change func(*Article, Version) error) (Result, error) {
	id, version, err := parseIdentity(idRaw, versionRaw)
	if err != nil {
		return Result{}, validation(err)
	}
	if err := authorize(service.authorizer, ctx, action, "article", id.Int64()); err != nil {
		return Result{}, err
	}
	var out Result
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx}
		value, err := repo.Find(ctx, id)
		if err != nil {
			return err
		}
		if err := change(value, version); err != nil {
			return err
		}
		if err := repo.Save(ctx, value); err != nil {
			return err
		}
		out = result(value)
		return nil
	})
	return out, stable(err)
}

// Delete soft-deletes an article after version and authorization checks.
func (service *Service) Delete(ctx context.Context, input Delete) error {
	id, version, err := parseIdentity(input.ID, input.Version)
	if err != nil {
		return validation(err)
	}
	if err := authorize(service.authorizer, ctx, ActionDeleteArticle, "article", id.Int64()); err != nil {
		return err
	}
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx}
		value, err := repo.Find(ctx, id)
		if err != nil {
			return err
		}
		if err := value.Delete(version, service.clock); err != nil {
			return err
		}
		return repo.Save(ctx, value)
	})
	return stable(err)
}

func authorize(authorizer content.Authorizer, ctx context.Context, action content.Action, kind string, id int64) error {
	if err := authorizer.Authorize(ctx, action, content.Resource{Kind: kind, ID: id}); err != nil {
		return permission()
	}
	return nil
}

func applyPatch(value *Article, version Version, input Patch, clock Clock) error {
	typeID, title, slug := value.ArticleTypeID(), value.Title(), value.Slug()
	digest, body, tags := value.Digest(), value.Content(), value.TagIDs()
	var err error
	if input.ArticleTypeID != nil {
		typeID, err = NewArticleTypeID(*input.ArticleTypeID)
		if err != nil {
			return err
		}
	}
	if input.Title != nil {
		title, err = NewTitle(*input.Title)
		if err != nil {
			return err
		}
	}
	if input.Slug != nil {
		slug, err = NewSlug(*input.Slug)
		if err != nil {
			return err
		}
	}
	if input.Digest != nil {
		digest, err = NewDigest(*input.Digest)
		if err != nil {
			return err
		}
	}
	if input.Content != nil {
		body, err = NewContent(*input.Content)
		if err != nil {
			return err
		}
	}
	if input.TagIDs != nil {
		tags, err = parseTags(*input.TagIDs)
		if err != nil {
			return err
		}
	}
	return value.Revise(version, ArticleRevision{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: tags}, clock)
}

var _ API = (*Service)(nil)
