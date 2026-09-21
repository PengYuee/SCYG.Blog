package article

import "context"

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
