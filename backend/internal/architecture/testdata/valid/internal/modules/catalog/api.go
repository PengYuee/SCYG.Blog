// Package catalog exposes protocol-neutral module input and output types.
package catalog

// ActionReadArticle is an immutable authorization action.
const ActionReadArticle = "content.article.read"

// FindArticle is a protocol-neutral article query.
type FindArticle struct{ ID int64 }

// ArticleResult is a protocol-neutral query result.
type ArticleResult struct{ Title string }

// ErrorCode identifies a stable application failure.
type ErrorCode string

// ApplicationError is a protocol-neutral typed failure.
type ApplicationError struct{ Code ErrorCode }

// Authorizer is a legal single-purpose module-root port.
type Authorizer interface{ Authorize(Action) error }

// Action identifies one protocol-neutral authorization decision.
type Action string
