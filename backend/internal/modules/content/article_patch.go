package content

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// PatchArticle applies partial article values and a supported lifecycle transition atomically.
func (module *Module) PatchArticle(ctx context.Context, command PatchArticle) (ArticleResult, error) {
	id, version, err := parseIdentity(command.ID, command.Version)
	if err != nil {
		return ArticleResult{}, validation(err)
	}
	if command.Status != nil && *command.Status != string(domain.StatusPublished) && *command.Status != string(domain.StatusArchived) {
		return ArticleResult{}, validation(invalidCommand("status"))
	}
	action := ActionReviseArticle
	if command.Status != nil && *command.Status == string(domain.StatusPublished) {
		action = ActionPublishArticle
	}
	if command.Status != nil && *command.Status == string(domain.StatusArchived) {
		action = ActionArchiveArticle
	}
	if err = module.authorizer.Authorize(ctx, action, Resource{Kind: "article", ID: command.ID}); err != nil {
		return ArticleResult{}, permission(err)
	}
	var result ArticleResult
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		article, findErr := transaction.Articles().Find(transactionContext, id)
		if findErr != nil {
			return findErr
		}
		if patchErr := module.applyArticlePatch(article, version, command); patchErr != nil {
			return patchErr
		}
		if saveErr := transaction.Articles().Save(transactionContext, article); saveErr != nil {
			return saveErr
		}
		result = articleResult(article)
		return nil
	})
	if err != nil {
		return ArticleResult{}, stable(err)
	}
	return result, nil
}

func (module *Module) applyArticlePatch(article *domain.Article, version domain.Version, command PatchArticle) error {
	if command.Status != nil {
		switch *command.Status {
		case string(domain.StatusPublished):
			return article.Publish(version, module.clock)
		case string(domain.StatusArchived):
			return article.Archive(version, module.clock)
		}
	}
	typeID, title, slug := article.ArticleTypeID(), article.Title(), article.Slug()
	digest, body, tags := article.Digest(), article.Content(), article.TagIDs()
	var err error
	if command.ArticleTypeID != nil {
		typeID, err = domain.NewArticleTypeID(*command.ArticleTypeID)
		if err != nil {
			return err
		}
	}
	if command.Title != nil {
		title, err = domain.NewTitle(*command.Title)
		if err != nil {
			return err
		}
	}
	if command.Slug != nil {
		slug, err = domain.NewSlug(*command.Slug)
		if err != nil {
			return err
		}
	}
	if command.Digest != nil {
		digest, err = domain.NewDigest(*command.Digest)
		if err != nil {
			return err
		}
	}
	if command.Content != nil {
		body, err = domain.NewContent(*command.Content)
		if err != nil {
			return err
		}
	}
	if command.TagIDs != nil {
		tags, err = parseTags(*command.TagIDs)
		if err != nil {
			return err
		}
	}
	return article.Revise(version, domain.ArticleRevision{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: tags}, module.clock)
}
