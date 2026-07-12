package content

import "testing"

func Test_ContentREST_constructor_nil_detection_covers_nilable_kinds(t *testing.T) {
	// Given
	var pointer *testNilValue
	var mapping map[string]string
	var values []string
	var function func()
	var channel chan string
	tests := []any{pointer, mapping, values, function, channel}

	// When / Then
	for _, value := range tests {
		if !nilService(value) {
			t.Fatalf("nilService(%T) = false", value)
		}
	}
	if nilService(testNilValue{}) {
		t.Fatal("nilService(non-nil value) = true")
	}
}

type testNilValue struct{}
