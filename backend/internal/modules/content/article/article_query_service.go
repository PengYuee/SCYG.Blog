package article

import (
	"context"

	"gorm.io/gorm"
)

// Get reads one published article through the feature service.
func (service *Service) Get(ctx context.Context, input Get) (Result, error) {
	return service.query.Get(ctx, input)
}

// GetManage reads one non-deleted article for authorized management clients.
func (service *Service) GetManage(ctx context.Context, input Get) (Result, error) {
	if err := authorize(service.authorizer, ctx, ActionReviseArticle, "article", input.ID); err != nil {
		return Result{}, err
	}
	return service.query.GetManage(ctx, input)
}

// ListManage reads non-deleted articles for authorized management clients.
func (service *Service) ListManage(ctx context.Context, input List) (Page, error) {
	if err := authorize(service.authorizer, ctx, ActionReviseArticle, "article", 0); err != nil {
		return Page{}, err
	}
	return service.query.ListManage(ctx, input)
}

// List reads published articles through the feature service.
func (service *Service) List(ctx context.Context, input List) (Page, error) {
	return service.query.List(ctx, input)
}

// GetManageInTx reads current state with authorization on the caller's connection.
func (service *Service) GetManageInTx(ctx context.Context, tx *gorm.DB, input Get) (Result, error) {
	if err := authorize(service.authorizer, ctx, ActionReviseArticle, "article", input.ID); err != nil {
		return Result{}, err
	}
	return (&Query{db: tx}).GetManage(ctx, input)
}

// GetInTx reads a published article on the caller-owned snapshot.
func (service *Service) GetInTx(ctx context.Context, tx *gorm.DB, input Get) (Result, error) {
	return (&Query{db: tx}).Get(ctx, input)
}

// ListInTx reads published articles on the caller-owned snapshot.
func (service *Service) ListInTx(ctx context.Context, tx *gorm.DB, input List) (Page, error) {
	return (&Query{db: tx}).List(ctx, input)
}

// ListManageInTx authorizes and reads management articles on the caller-owned snapshot.
func (service *Service) ListManageInTx(ctx context.Context, tx *gorm.DB, input List) (Page, error) {
	if err := authorize(service.authorizer, ctx, ActionReviseArticle, "article", 0); err != nil {
		return Page{}, err
	}
	return (&Query{db: tx}).ListManage(ctx, input)
}
