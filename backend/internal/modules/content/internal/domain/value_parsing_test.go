package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func Test_Article_values_parse_valid_and_invalid_inputs(t *testing.T) {
	cases := []struct {
		name  string
		parse func() error
		valid bool
	}{
		{"valid title", func() error { _, err := domain.NewTitle("  Hello  "); return err }, true},
		{"empty title", func() error { _, err := domain.NewTitle(" "); return err }, false},
		{"long title", func() error { _, err := domain.NewTitle(strings.Repeat("x", 201)); return err }, false},
		{"valid slug", func() error { _, err := domain.NewSlug("hello-world"); return err }, true},
		{"invalid slug", func() error { _, err := domain.NewSlug("Hello World!"); return err }, false},
		{"zero version", func() error { _, err := domain.NewVersion(0); return err }, false},
		{"positive version", func() error { _, err := domain.NewVersion(1); return err }, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.parse()
			if testCase.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !testCase.valid && !errors.Is(err, domain.ErrInvalidValue) {
				t.Fatalf("expected invalid value, got %v", err)
			}
		})
	}
}

func Fuzz_Article_Title_constructor_is_deterministic(fuzz *testing.F) {
	fuzz.Add("hello")
	fuzz.Add("")
	fuzz.Add(strings.Repeat("x", 201))
	fuzz.Fuzz(func(t *testing.T, raw string) {
		first, firstErr := domain.NewTitle(raw)
		second, secondErr := domain.NewTitle(raw)
		if first != second || ((firstErr == nil) != (secondErr == nil)) {
			t.Fatal("constructor is nondeterministic")
		}
	})
}
