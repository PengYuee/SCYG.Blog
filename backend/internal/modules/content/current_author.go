package content

import "context"

// InvalidAuthorIDError 表示作者标识不符合 32 位小写十六进制契约。
type InvalidAuthorIDError struct{}

// Error 返回稳定的中文作者标识错误。
func (InvalidAuthorIDError) Error() string { return "作者标识必须为 32 位小写十六进制" }

// CurrentAuthorUnavailableError 表示当前运行环境没有可信作者身份来源。
type CurrentAuthorUnavailableError struct{}

// Error 返回稳定的中文身份不可用错误。
func (CurrentAuthorUnavailableError) Error() string { return "当前作者身份不可用" }

// AuthorID 是经过解析的 32 位小写十六进制作者标识。
type AuthorID struct{ value string }

// NewAuthorID 在信任边界解析作者标识。
func NewAuthorID(raw string) (AuthorID, error) {
	if !validAuthorID(raw) {
		return AuthorID{}, InvalidAuthorIDError{}
	}
	return AuthorID{value: raw}, nil
}

// String 返回经过验证的作者标识。
func (authorID AuthorID) String() string { return authorID.value }

// CurrentAuthorProvider 从可信进程依赖中提供当前作者；HTTP 输入不得实现该端口。
type CurrentAuthorProvider interface {
	CurrentAuthor(context.Context) (AuthorID, error)
}

// FixedCurrentAuthorProvider 是仅供 development 组合根使用的固定作者实现。
type FixedCurrentAuthorProvider struct{ authorID AuthorID }

// NewFixedCurrentAuthorProvider 构造跨请求稳定的开发作者提供者。
func NewFixedCurrentAuthorProvider(authorID AuthorID) FixedCurrentAuthorProvider {
	return FixedCurrentAuthorProvider{authorID: authorID}
}

// CurrentAuthor 返回构造时固定且已验证的作者标识。
func (provider FixedCurrentAuthorProvider) CurrentAuthor(context.Context) (AuthorID, error) {
	return provider.authorID, nil
}

// CurrentAuthorProviderOrUnavailable 将 nil 或带类型 nil 安全降级为不可用实现。
func CurrentAuthorProviderOrUnavailable(candidate CurrentAuthorProvider) CurrentAuthorProvider {
	if nilLike(candidate) {
		return unavailableCurrentAuthorProvider{}
	}
	return candidate
}

type unavailableCurrentAuthorProvider struct{}

func (unavailableCurrentAuthorProvider) CurrentAuthor(context.Context) (AuthorID, error) {
	return AuthorID{}, CurrentAuthorUnavailableError{}
}

func validAuthorID(raw string) bool {
	if len(raw) != 32 {
		return false
	}
	for index := range len(raw) {
		character := raw[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
