package content

import "testing"

func Test_IfNoneMatch_uses_weak_comparison_for_lists_and_wildcard(t *testing.T) {
	tag := `"abc"`
	for _, value := range []string{`"abc"`, `W/"abc"`, ` "other" , W/"abc" `, `"other,value", W/"abc"`, `W/"abc", "last"`, `"first", W/"abc"`, "  \t\"first\" ,  W/\"abc\"  ", `*`} {
		if !ifNoneMatch(value, tag) {
			t.Fatalf("未匹配 %q", value)
		}
	}
}

func Test_IfNoneMatch_accepts_comma_and_backslash_as_literal_etagc(t *testing.T) {
	if !ifNoneMatch(`W/"path\part,value"`, `"path\part,value"`) {
		t.Fatal("合法 etagc 未按弱比较匹配")
	}
}

func Test_IfNoneMatch_rejects_malformed_values_without_matching(t *testing.T) {
	tag := `"abc"`
	for _, value := range []string{"", "W/abc", `"abc`, `abc"`, `W/ W/"abc"`, `"other",broken`, `*, "abc"`, `,"abc"`, `"abc",`, `"abc",,W/"abc"`} {
		if ifNoneMatch(value, tag) {
			t.Fatalf("畸形值误匹配 %q", value)
		}
	}
}
