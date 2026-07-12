package reviewtest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// scenarioRequirement 定义一个 E2E 场景不可缺少的语义符号。
type scenarioRequirement struct {
	// Name 是精确测试函数名。
	Name string
	// RequiredSymbols 是 helper、状态常量和比较调用集合。
	RequiredSymbols []string
}

// validateScenario 是生产审查与 mutation fixtures 共用的纯 AST validator。
func validateScenario(file *ast.File, requirement scenarioRequirement) []error {
	var target *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == requirement.Name {
			target = function
			break
		}
	}
	if target == nil {
		return []error{fmt.Errorf("缺少 E2E 场景 %s", requirement.Name)}
	}
	symbols := functionSymbolSet(target)
	failures := make([]error, 0)
	for _, required := range requirement.RequiredSymbols {
		if !symbols[required] {
			failures = append(failures, fmt.Errorf("E2E %s 缺少关键 helper/status/comparison %s", requirement.Name, required))
		}
	}
	return failures
}

func scenarioRequirements() []scenarioRequirement {
	return []scenarioRequirement{
		{Name: "Test_E2E_migrations_roundtrip", RequiredSymbols: []string{"newHarness", "Shutdown", "migrateDown", "migrateUp", "QueryRowContext", "Scan", "Fatalf"}},
		{Name: "Test_E2E_scalar_is_offline_and_self_hosted", RequiredSymbols: []string{"newHarness", "request", "assertLocalReferences", "StatusOK", "Fatalf"}},
		{Name: "Test_E2E_public_reads_hide_drafts", RequiredSymbols: []string{"newHarness", "createContent", "createArticle", "request", "StatusNotFound", "StatusOK", "Fatalf"}},
		{Name: "Test_E2E_allow_all_performs_real_crud", RequiredSymbols: []string{"newHarness", "createContent", "createArticle", "MethodDelete", "StatusNoContent", "Fatalf"}},
		{Name: "Test_E2E_production_denies_writes", RequiredSymbols: []string{"newHarness", "newHarnessWithDatabase", "resourceSeed", "deniedWrites", "snapshotDatabase", "assertForbiddenProblem", "DeepEqual", "StatusForbidden", "Fatalf"}},
		{Name: "Test_E2E_stale_etag_is_rejected", RequiredSymbols: []string{"newHarness", "snapshotTag", "request", "DeepEqual", "StatusPreconditionFailed", "Fatalf"}},
		{Name: "Test_E2E_readiness_fails_during_database_outage", RequiredSymbols: []string{"newHarness", "setDatabaseConnectionsAllowed", "waitHTTPStatus", "StatusServiceUnavailable", "StatusOK", "Fatalf"}},
		{Name: "Test_E2E_restart_preserves_committed_data", RequiredSymbols: []string{"newHarness", "Shutdown", "start", "Contains", "Fatalf"}},
		{Name: "Test_E2E_sigterm_closes_runtime", RequiredSymbols: []string{"assertSignalSubprocessShutdown"}},
	}
}
func criticalScenarioRequirements() []scenarioRequirement {
	return []scenarioRequirement{
		{Name: "Test_E2E_scalar_is_offline_and_self_hosted", RequiredSymbols: []string{"assertLocalReferences", "StatusOK"}},
		{Name: "Test_E2E_production_denies_writes", RequiredSymbols: []string{"snapshotDatabase", "DeepEqual", "StatusForbidden"}},
		{Name: "Test_E2E_stale_etag_is_rejected", RequiredSymbols: []string{"snapshotTag", "DeepEqual", "StatusPreconditionFailed"}},
		{Name: "Test_E2E_readiness_fails_during_database_outage", RequiredSymbols: []string{"setDatabaseConnectionsAllowed", "waitHTTPStatus", "StatusServiceUnavailable", "StatusOK"}},
		{Name: "Test_E2E_sigterm_closes_runtime", RequiredSymbols: []string{"assertSignalSubprocessShutdown"}},
	}
}

func parseFixture(t *testing.T, source string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "mutation_test.go", source, 0)
	if err != nil {
		t.Fatalf("解析 mutation fixture 失败：%v", err)
	}
	return file
}

func Test_ReviewE2E_validator_rejects_each_mutated_semantic(t *testing.T) {
	fixtures := map[string]string{
		"Test_E2E_scalar_is_offline_and_self_hosted":      `func Test_E2E_scalar_is_offline_and_self_hosted(){ assertLocalReferences(); _ = http.StatusOK }`,
		"Test_E2E_production_denies_writes":               `func Test_E2E_production_denies_writes(){ snapshotDatabase(); reflect.DeepEqual(); _ = http.StatusForbidden }`,
		"Test_E2E_stale_etag_is_rejected":                 `func Test_E2E_stale_etag_is_rejected(){ snapshotTag(); reflect.DeepEqual(); _ = http.StatusPreconditionFailed }`,
		"Test_E2E_readiness_fails_during_database_outage": `func Test_E2E_readiness_fails_during_database_outage(){ setDatabaseConnectionsAllowed(); waitHTTPStatus(); _ = http.StatusServiceUnavailable; _ = http.StatusOK }`,
		"Test_E2E_sigterm_closes_runtime":                 `func Test_E2E_sigterm_closes_runtime(){ assertSignalSubprocessShutdown() }`,
	}
	for _, requirement := range criticalScenarioRequirements() {
		for _, removed := range requirement.RequiredSymbols {
			t.Run(requirement.Name+"/删除_"+removed, func(t *testing.T) {
				mutated := strings.Replace(fixtures[requirement.Name], removed, "removedSymbol", 1)
				file := parseFixture(t, "package fixture\n"+mutated)
				if failures := validateScenario(file, requirement); len(failures) == 0 {
					t.Fatalf("删除 %s 后 validator 错误放行", removed)
				}
			})
		}
	}
}
