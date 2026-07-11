package content_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

type pointerAuthorizer struct{}

func (*pointerAuthorizer) Authorize(context.Context, content.Action, content.Resource) error {
	return nil
}

type mapAuthorizer map[string]bool

func (mapAuthorizer) Authorize(context.Context, content.Action, content.Resource) error { return nil }

type sliceAuthorizer []string

func (sliceAuthorizer) Authorize(context.Context, content.Action, content.Resource) error { return nil }

type funcAuthorizer func() error

func (authorizer funcAuthorizer) Authorize(context.Context, content.Action, content.Resource) error {
	return authorizer()
}

type chanAuthorizer chan struct{}

func (chanAuthorizer) Authorize(context.Context, content.Action, content.Resource) error { return nil }

type valueAuthorizer struct{}

func (valueAuthorizer) Authorize(context.Context, content.Action, content.Resource) error { return nil }

func Test_AuthorizerOrDeny_normalizes_every_typed_nil_kind(t *testing.T) {
	var pointer *pointerAuthorizer
	var mapped mapAuthorizer
	var sliced sliceAuthorizer
	var function funcAuthorizer
	var channel chanAuthorizer
	var interfaced content.Authorizer
	cases := []struct {
		name      string
		candidate content.Authorizer
	}{
		{"invalid interface", interfaced}, {"pointer", pointer}, {"map", mapped}, {"slice", sliced}, {"func", function}, {"chan", channel},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized := content.AuthorizerOrDeny(testCase.candidate)
			err := normalized.Authorize(context.Background(), content.ActionCreateArticle, content.Resource{})
			if !errors.Is(err, content.ErrPermissionDenied) {
				t.Fatalf("typed nil %s was not denied: %v", testCase.name, err)
			}
		})
	}
}

func Test_AuthorizerOrDeny_preserves_non_nil_implementations(t *testing.T) {
	cases := []struct {
		name      string
		candidate content.Authorizer
	}{
		{"empty struct", valueAuthorizer{}}, {"pointer", &pointerAuthorizer{}}, {"map", mapAuthorizer{}}, {"slice", sliceAuthorizer{}}, {"func", funcAuthorizer(func() error { return nil })}, {"chan", make(chanAuthorizer)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized := content.AuthorizerOrDeny(testCase.candidate)
			if err := normalized.Authorize(context.Background(), content.ActionCreateArticle, content.Resource{}); err != nil {
				t.Fatalf("non-nil %s was replaced: %v", testCase.name, err)
			}
		})
	}
}

func Test_AuthorizerOrDeny_is_race_free_under_concurrent_calls(t *testing.T) {
	candidates := []content.Authorizer{mapAuthorizer(nil), sliceAuthorizer(nil), funcAuthorizer(nil), valueAuthorizer{}}
	var wait sync.WaitGroup
	for range 32 {
		for _, candidate := range candidates {
			wait.Go(func() {
				normalized := content.AuthorizerOrDeny(candidate)
				_ = normalized.Authorize(context.Background(), content.ActionCreateArticle, content.Resource{})
			})
		}
	}
	wait.Wait()
}
