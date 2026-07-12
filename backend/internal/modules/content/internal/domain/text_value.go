package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
func (value Title) valid() bool   { return validResponseText(value.value, 120) }
func (value Slug) valid() bool {
	matched, _ := regexp.MatchString(`^[a-z0-9]+(?:-[a-z0-9]+)*$`, value.value)
	return utf8.ValidString(value.value) && len([]rune(value.value)) <= 160 && matched
}
func (value Digest) valid() bool  { return validResponseText(value.value, 500) }
func (value Content) valid() bool { return validResponseText(value.value, 0) }
func (value Name) valid() bool    { return validResponseText(value.value, 60) }
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
