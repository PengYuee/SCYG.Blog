package content_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_AuthorID_rejects_non_32_hex_values(t *testing.T) {
	tests := []string{"", "0123456789abcdef0123456789abcde", "0123456789abcdef0123456789abcdef0", "0123456789abcdef0123456789abcdeg"}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			// When
			_, err := content.NewAuthorID(raw)
			// Then
			if !errors.Is(err, content.InvalidAuthorIDError{}) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func Test_FixedCurrentAuthorProvider_returns_stable_author(t *testing.T) {
	// Given
	authorID, err := content.NewAuthorID("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	provider := content.NewFixedCurrentAuthorProvider(authorID)
	// When
	first, firstErr := provider.CurrentAuthor(context.Background())
	second, secondErr := provider.CurrentAuthor(context.Background())
	// Then
	if firstErr != nil || secondErr != nil || first != second || first.String() != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("first=%v second=%v errors=%v/%v", first, second, firstErr, secondErr)
	}
}

func Test_CurrentAuthorProviderOrUnavailable_denies_nil_provider(t *testing.T) {
	// When
	_, err := content.CurrentAuthorProviderOrUnavailable(nil).CurrentAuthor(context.Background())
	// Then
	if !errors.Is(err, content.CurrentAuthorUnavailableError{}) {
		t.Fatalf("err=%v", err)
	}
}

type nilCurrentAuthorProvider struct{}

func (*nilCurrentAuthorProvider) CurrentAuthor(context.Context) (content.AuthorID, error) {
	return content.AuthorID{}, nil
}

func Test_CurrentAuthorProviderOrUnavailable_denies_typed_nil_provider_with_stable_error(t *testing.T) {
	// Given
	var candidate *nilCurrentAuthorProvider

	// When
	_, err := content.CurrentAuthorProviderOrUnavailable(candidate).CurrentAuthor(context.Background())

	// Then
	var unavailable content.CurrentAuthorUnavailableError
	if !errors.As(err, &unavailable) || !errors.Is(err, content.CurrentAuthorUnavailableError{}) {
		t.Fatalf("err=%T %v", err, err)
	}
	if err.Error() != "当前作者身份不可用" {
		t.Fatalf("message=%q", err.Error())
	}
}

func Test_NewAuthorID_returns_stable_typed_error(t *testing.T) {
	// When
	_, err := content.NewAuthorID("invalid")

	// Then
	var invalid content.InvalidAuthorIDError
	if !errors.As(err, &invalid) || !errors.Is(err, content.InvalidAuthorIDError{}) {
		t.Fatalf("err=%T %v", err, err)
	}
	if err.Error() != "作者标识必须为 32 位小写十六进制" {
		t.Fatalf("message=%q", err.Error())
	}
}
