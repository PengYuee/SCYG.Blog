package contracttest

import (
	"net/http"
	"strings"
	"testing"
)

func Test_OpenAPI_agent_paths_and_request_contracts(t *testing.T) {
	document := loadAuthoritativeSpec(t)
	all := operations(document)
	expected := map[string]string{
		"POST /api/v1/ai/search":           "agentSearch",
		"POST /api/v1/ai/write":            "agentWrite",
		"POST /api/v1/ai/polish":           "agentPolish",
		"POST /api/v1/ai/chat":             "agentChat",
		"POST /api/v1/public/ai/session":   "agentPublicSession",
		"POST /api/v1/public/ai/search":    "agentPublicSearch",
		"GET /api/v1/runs/{runId}":         "agentGet",
		"GET /api/v1/runs/{runId}/events":  "agentEvents",
		"POST /api/v1/runs/{runId}/resume": "agentResume",
		"POST /api/v1/runs/{runId}/cancel": "agentCancel",
	}
	for label, id := range expected {
		operation := all[label]
		if operation == nil || operation.OperationID != id {
			t.Fatalf("missing Agent operation %s (%s)", label, id)
		}
		for _, status := range []string{"400", "404", "429", "503", "500", "default"} {
			assertProblemResponse(t, operation, status)
		}
		if id == "agentPublicSession" || id == "agentCancel" {
			if operation.RequestBody != nil {
				t.Fatalf("%s must not accept a body", id)
			}
		}
		if strings.HasPrefix(label, http.MethodPost+" ") && id != "agentPublicSession" {
			found := false
			for _, ref := range operation.Parameters {
				if ref.Ref == "#/components/parameters/AgentIdempotencyKey" && ref.Value != nil && ref.Value.Required {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s must require shared UUID v4 Idempotency-Key", id)
			}
		}
	}
	for label, operation := range all {
		if !strings.HasPrefix(operation.OperationID, "agent") {
			continue
		}
		if expected[label] != operation.OperationID {
			t.Fatalf("unexpected Agent operation %s (%s)", label, operation.OperationID)
		}
	}
	search := all["POST /api/v1/public/ai/search"]
	if search.RequestBody == nil || search.RequestBody.Value == nil || !search.RequestBody.Value.Required {
		t.Fatal("public search must require a JSON body")
	}
	media := search.RequestBody.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Ref != "#/components/schemas/SearchInput" {
		t.Fatal("public search must use SearchInput")
	}
	for _, status := range []string{"401", "409", "413", "415"} {
		assertProblemResponse(t, search, status)
	}
	if response(t, search, "202").Content.Get("application/json").Schema.Ref != "#/components/schemas/AgentRun" {
		t.Fatal("public search must return the shared AgentRun")
	}
	session := all["POST /api/v1/public/ai/session"]
	key := parameterSchema(t, document, "AgentIdempotencyKey")
	if err := key.VisitJSON("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatalf("valid UUID v4 key rejected: %v", err)
	}
	for _, invalid := range []string{"", "not-a-uuid", "550e8400-e29b-11d4-a716-446655440000", "550e8400-e29b-41d4-7716-446655440000"} {
		if err := key.VisitJSON(invalid); err == nil {
			t.Fatalf("Idempotency-Key accepted %q", invalid)
		}
	}
	if response(t, session, "200").Content.Get("application/json").Schema.Ref != "#/components/schemas/PublicSearchSession" {
		t.Fatal("public session must return PublicSearchSession")
	}
}

func Test_OpenAPI_public_search_schema_behavior(t *testing.T) {
	document := loadAuthoritativeSpec(t)
	assertExactProperties(t, document, "PublicSearchSession", []string{"accessToken", "expiresAt", "queryMaxChars", "maxResults"})
	assertSchemaRequired(t, document, "PublicSearchSession", []string{"accessToken", "expiresAt", "queryMaxChars", "maxResults"})
	session := document.Components.Schemas["PublicSearchSession"].Value
	if session.Properties["expiresAt"].Value.Format != "date-time" {
		t.Fatal("session expiresAt must be date-time")
	}
	valid := map[string]any{"accessToken": "opaque", "expiresAt": "2026-10-10T12:00:00Z", "queryMaxChars": float64(2000), "maxResults": float64(5)}
	if err := session.VisitJSON(valid); err != nil {
		t.Fatalf("valid session rejected: %v", err)
	}
	for _, missing := range session.Required {
		invalid := make(map[string]any, len(valid))
		for key, value := range valid {
			if key != missing {
				invalid[key] = value
			}
		}
		if err := session.VisitJSON(invalid); err == nil {
			t.Fatalf("session accepted missing %s", missing)
		}
	}
	assertExactProperties(t, document, "SearchInput", []string{"query", "max_results"})
	assertSchemaRequired(t, document, "SearchInput", []string{"query"})
	search := document.Components.Schemas["SearchInput"].Value
	if search.Properties["max_results"].Value.Default != float64(5) {
		t.Fatal("SearchInput max_results must default to 5")
	}
	for _, validInput := range []any{map[string]any{"query": "Go"}, map[string]any{"query": "Go", "max_results": float64(20)}} {
		if err := search.VisitJSON(validInput); err != nil {
			t.Fatalf("valid search rejected: %v", err)
		}
	}
	for _, invalid := range []any{
		nil, "Go",
		map[string]any{},
		map[string]any{"query": ""},
		map[string]any{"query": strings.Repeat("x", 16001)},
		map[string]any{"query": "Go", "max_results": float64(0)},
		map[string]any{"query": "Go", "max_results": float64(21)},
		map[string]any{"query": "Go", "max_results": float64(1.5)},
		map[string]any{"query": "Go", "maxResults": float64(5)},
		map[string]any{"query": "Go", "contentScope": "author"},
	} {
		if err := search.VisitJSON(invalid); err == nil {
			t.Fatalf("SearchInput accepted invalid value %#v", invalid)
		}
	}
	assertExactProperties(t, document, "AgentRun", []string{"runId", "status", "capability", "recipeId", "recipeVersion", "createdAt", "updatedAt", "failure", "result", "pendingInteraction", "streamUrl"})
}

func Test_OpenAPI_agent_retry_after_contract(t *testing.T) {
	document := loadAuthoritativeSpec(t)
	for _, operation := range operations(document) {
		if !strings.HasPrefix(operation.OperationID, "agent") {
			continue
		}
		limited := response(t, operation, "429")
		header := limited.Headers["Retry-After"]
		if header == nil || header.Value == nil || header.Value.Required || header.Value.Schema == nil || header.Value.Schema.Value == nil {
			t.Fatalf("%s must declare optional Retry-After on 429", operation.OperationID)
		}
		schema := header.Value.Schema.Value
		for _, valid := range []string{"0", "1", "60", "3600"} {
			if err := schema.VisitJSON(valid); err != nil {
				t.Fatalf("Retry-After rejected %q: %v", valid, err)
			}
		}
		for _, invalid := range []string{"", "-1", "+1", "1.5", "1e3", " 1", "Wed, 21 Oct 2015 07:28:00 GMT"} {
			if err := schema.VisitJSON(invalid); err == nil {
				t.Fatalf("Retry-After accepted %q", invalid)
			}
		}
		media := limited.Content.Get("application/problem+json")
		if media == nil || media.Schema == nil || media.Schema.Ref != "#/components/schemas/Problem" {
			t.Fatalf("%s must preserve Problem on 429", operation.OperationID)
		}
	}
}
