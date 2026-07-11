package domain

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// ArticleID is a parsed positive article identifier.
type ArticleID struct{ value int64 }

// ArticleTypeID is a parsed positive article-type identifier.
type ArticleTypeID struct{ value int64 }

// TagID is a parsed positive tag identifier.
type TagID struct{ value int64 }

// Title is a trimmed article title of at most 200 runes.
type Title struct{ value string }

// Slug is a lowercase, hyphen-separated article slug.
type Slug struct{ value string }

// Digest is a trimmed article summary of at most 500 runes.
type Digest struct{ value string }

// Content is non-empty article body content.
type Content struct{ value string }

// Name is a trimmed taxonomy name of at most 100 runes.
type Name struct{ value string }

// Version is a strictly positive optimistic-concurrency version.
type Version struct{ value uint64 }

// NewArticleID parses a positive article identifier.
func NewArticleID(raw int64) (ArticleID, error) {
	if raw <= 0 {
		return ArticleID{}, invalid("article_id")
	}
	return ArticleID{raw}, nil
}

// NewArticleTypeID parses a positive article-type identifier.
func NewArticleTypeID(raw int64) (ArticleTypeID, error) {
	if raw <= 0 {
		return ArticleTypeID{}, invalid("article_type_id")
	}
	return ArticleTypeID{raw}, nil
}

// NewTagID parses a positive tag identifier.
func NewTagID(raw int64) (TagID, error) {
	if raw <= 0 {
		return TagID{}, invalid("tag_id")
	}
	return TagID{raw}, nil
}

// NewTitle parses and trims an article title.
func NewTitle(raw string) (Title, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len([]rune(value)) > 200 {
		return Title{}, invalid("title")
	}
	return Title{value}, nil
}

// NewSlug parses and normalizes an article slug.
func NewSlug(raw string) (Slug, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	matched, err := regexp.MatchString(`^[a-z0-9]+(?:-[a-z0-9]+)*$`, value)
	if err != nil || len(value) > 200 || !matched {
		return Slug{}, invalid("slug")
	}
	return Slug{value}, nil
}

// NewDigest parses and trims an article digest.
func NewDigest(raw string) (Digest, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len([]rune(value)) > 500 {
		return Digest{}, invalid("digest")
	}
	return Digest{value}, nil
}

// NewContent parses non-empty article content.
func NewContent(raw string) (Content, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Content{}, invalid("content")
	}
	return Content{value}, nil
}

// NewName parses and trims a taxonomy name.
func NewName(raw string) (Name, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len([]rune(value)) > 100 {
		return Name{}, invalid("name")
	}
	return Name{value}, nil
}

// NewVersion parses a strictly positive version.
func NewVersion(raw uint64) (Version, error) {
	if raw == 0 {
		return Version{}, invalid("version")
	}
	return Version{raw}, nil
}
func initialVersion() Version { return Version{1} }
func (version Version) next() (Version, error) {
	if version.value == math.MaxUint64 {
		return Version{}, ErrVersionExhausted
	}
	return Version{version.value + 1}, nil
}
func invalid(field string) error { return fmt.Errorf("%s: %w", field, ErrInvalidValue) }

// Int64 returns the article identifier value.
func (id ArticleID) Int64() int64 { return id.value }

// Int64 returns the article-type identifier value.
func (id ArticleTypeID) Int64() int64 { return id.value }

// Int64 returns the tag identifier value.
func (id TagID) Int64() int64 { return id.value }

// String returns the parsed title.
func (value Title) String() string { return value.value }

// String returns the parsed slug.
func (value Slug) String() string { return value.value }

// String returns the parsed digest.
func (value Digest) String() string { return value.value }

// String returns the parsed content.
func (value Content) String() string { return value.value }

// String returns the parsed name.
func (value Name) String() string { return value.value }

// Uint64 returns the optimistic-concurrency version.
func (version Version) Uint64() uint64 { return version.value }

func (id ArticleID) valid() bool     { return id.value > 0 }
func (id ArticleTypeID) valid() bool { return id.value > 0 }
func (id TagID) valid() bool         { return id.value > 0 }
func (value Title) valid() bool      { return value.value != "" && len([]rune(value.value)) <= 200 }
func (value Slug) valid() bool {
	matched, _ := regexp.MatchString(`^[a-z0-9]+(?:-[a-z0-9]+)*$`, value.value)
	return len(value.value) <= 200 && matched
}
func (value Digest) valid() bool    { return value.value != "" && len([]rune(value.value)) <= 500 }
func (value Content) valid() bool   { return value.value != "" }
func (value Name) valid() bool      { return value.value != "" && len([]rune(value.value)) <= 100 }
func (version Version) valid() bool { return version.value > 0 }
