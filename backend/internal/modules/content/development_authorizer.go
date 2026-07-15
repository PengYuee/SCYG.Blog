package content

import "context"

// DevelopmentAuthorizer 是仅由 development 组合根为已验证固定作者构造的写入授权器。
type DevelopmentAuthorizer struct{ authorID AuthorID }

// NewDevelopmentAuthorizer 绑定已验证固定作者并构造开发授权器。
func NewDevelopmentAuthorizer(authorID AuthorID) DevelopmentAuthorizer {
	return DevelopmentAuthorizer{authorID: authorID}
}

// Authorize 仅代表组合根已验证的固定开发作者通过授权。
func (authorizer DevelopmentAuthorizer) Authorize(ctx context.Context, action Action, resource Resource) error {
	if authorizer.authorID.String() == "" {
		return DenyAll{}.Authorize(ctx, action, resource)
	}
	return nil
}
