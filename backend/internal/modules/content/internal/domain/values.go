package domain

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ArticleID 是已解析的正文章标识。
type ArticleID struct{ value int64 }

// ArticleTypeID 是已解析的正文章分类标识。
type ArticleTypeID struct{ value int64 }

// TagID 是已解析的正标签标识。
type TagID struct{ value int64 }

// Title 是去除首尾空白且长度受限的文章标题。
type Title struct{ value string }

// Slug 是小写并以连字符分隔的文章 URL 标识。
type Slug struct{ value string }

// Digest 是去除首尾空白且长度受限的文章摘要。
type Digest struct{ value string }

// Content 是非空文章正文。
type Content struct{ value string }

// Name 是去除首尾空白且长度受限的分类名称。
type Name struct{ value string }

// Version 是严格为正的乐观并发版本。
type Version struct{ value uint64 }

// NewArticleID 将正整数解析为文章标识。
func NewArticleID(raw int64) (ArticleID, error) {
	if raw <= 0 {
		return ArticleID{}, invalid("article_id")
	}
	return ArticleID{raw}, nil
}

// NewArticleTypeID 将正整数解析为文章分类标识。
func NewArticleTypeID(raw int64) (ArticleTypeID, error) {
	if raw <= 0 {
		return ArticleTypeID{}, invalid("article_type_id")
	}
	return ArticleTypeID{raw}, nil
}

// NewTagID 将正整数解析为标签标识。
func NewTagID(raw int64) (TagID, error) {
	if raw <= 0 {
		return TagID{}, invalid("tag_id")
	}
	return TagID{raw}, nil
}

// NewTitle 解析并去除文章标题首尾空白。
func NewTitle(raw string) (Title, error) {
	value := strings.TrimSpace(raw)
	if !validResponseText(value, 120) {
		return Title{}, invalid("title")
	}
	return Title{value}, nil
}

// NewSlug 解析并规范化文章 URL 标识。
func NewSlug(raw string) (Slug, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	matched, err := regexp.MatchString(`^[a-z0-9]+(?:-[a-z0-9]+)*$`, value)
	if err != nil || !utf8.ValidString(value) || len([]rune(value)) > 160 || !matched {
		return Slug{}, invalid("slug")
	}
	return Slug{value}, nil
}

// NewDigest 解析并去除文章摘要首尾空白。
func NewDigest(raw string) (Digest, error) {
	value := strings.TrimSpace(raw)
	if !validResponseText(value, 500) {
		return Digest{}, invalid("digest")
	}
	return Digest{value}, nil
}

// NewContent 解析非空文章正文。
func NewContent(raw string) (Content, error) {
	value := strings.TrimSpace(raw)
	if !validResponseText(value, 0) {
		return Content{}, invalid("content")
	}
	return Content{value}, nil
}

// NewName 解析并去除分类名称首尾空白。
func NewName(raw string) (Name, error) {
	value := strings.TrimSpace(raw)
	if !validResponseText(value, 60) {
		return Name{}, invalid("name")
	}
	return Name{value}, nil
}

// NewVersion 解析严格为正的版本号。
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

// Int64 返回文章标识数值。
func (id ArticleID) Int64() int64 { return id.value }

// Int64 返回文章分类标识数值。
func (id ArticleTypeID) Int64() int64 { return id.value }

// Int64 返回标签标识数值。
func (id TagID) Int64() int64 { return id.value }

// String 返回已解析标题。
func (value Title) String() string { return value.value }

// String 返回已解析 URL 标识。
func (value Slug) String() string { return value.value }

// String 返回已解析摘要。
func (value Digest) String() string { return value.value }

// String 返回已解析正文。
func (value Content) String() string { return value.value }

// String 返回已解析名称。
func (value Name) String() string { return value.value }

// Uint64 返回乐观并发版本号。
func (version Version) Uint64() uint64 { return version.value }

func (id ArticleID) valid() bool     { return id.value > 0 }
func (id ArticleTypeID) valid() bool { return id.value > 0 }
func (id TagID) valid() bool         { return id.value > 0 }
func (value Title) valid() bool      { return validResponseText(value.value, 120) }
func (value Slug) valid() bool {
	matched, _ := regexp.MatchString(`^[a-z0-9]+(?:-[a-z0-9]+)*$`, value.value)
	return utf8.ValidString(value.value) && len([]rune(value.value)) <= 160 && matched
}
func (value Digest) valid() bool    { return validResponseText(value.value, 500) }
func (value Content) valid() bool   { return validResponseText(value.value, 0) }
func (value Name) valid() bool      { return validResponseText(value.value, 60) }
func (version Version) valid() bool { return version.value > 0 }

func validResponseText(value string, maximum int) bool {
	if value == "" || !utf8.ValidString(value) || (maximum > 0 && len([]rune(value)) > maximum) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
