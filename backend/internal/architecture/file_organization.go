package architecture

import (
	"path"
	"slices"
	"strings"
)

const (
	moduleFileNameCode        = "ARCH_MODULE_FILE_NAME"
	moduleLayerSubpackageCode = "ARCH_MODULE_LAYER_SUBPACKAGE"
	fileNameCorrectionDetail  = "；使用业务主体前缀和固定职责后缀"
	fileGrammarDetail         = "模块源码文件必须完整匹配 <subject>_<role>.go 或 <subject>_<role>_<subrole>.go 的有限职责语法"
	layerPackageDetail        = "模块 Go package 仅允许根、internal/domain、internal/application、internal/postgres 和 postgres；禁止额外子 package "
)

func checkFileOrganization(file sourceFile) []Violation {
	parts := strings.Split(file.relative, "/")
	if len(parts) < 4 || parts[0] != "internal" || parts[1] != "modules" {
		return nil
	}
	layer, extraPackage := moduleFileLayer(parts)
	if extraPackage != "" {
		return []Violation{{Code: moduleLayerSubpackageCode, Path: file.relative, Detail: layerPackageDetail + extraPackage}}
	}
	if violation, found := moduleFileNameViolation(file, layer); found {
		return []Violation{violation}
	}
	return nil
}

func moduleFileLayer(parts []string) (string, string) {
	if len(parts) == 4 {
		return "root", ""
	}
	if len(parts) == 5 && parts[3] == "postgres" {
		return "adapter", ""
	}
	if parts[3] == "postgres" && len(parts) > 5 {
		return "", parts[4]
	}
	if len(parts) == 6 && parts[3] == "internal" && slices.Contains([]string{"domain", "application", "postgres"}, parts[4]) {
		return parts[4], ""
	}
	if parts[3] == "internal" {
		if len(parts) > 4 && !slices.Contains([]string{"domain", "application", "postgres"}, parts[4]) {
			return "", parts[4]
		}
		if len(parts) > 6 {
			return "", parts[5]
		}
	}
	return "", parts[3]
}

func moduleFileNameViolation(file sourceFile, layer string) (Violation, bool) {
	name := path.Base(file.relative)
	if testStem, isTest := strings.CutSuffix(name, "_test.go"); isTest {
		return testFileNameViolation(file, testStem)
	}
	stem := strings.TrimSuffix(name, ".go")
	if violation, found := forbiddenGenericFileViolation(file, stem); found {
		return violation, true
	}
	if strings.HasSuffix(stem, "_record") {
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: "数据库数据模型文件必须使用 <subject>_model.go，禁止 *_record.go"}, true
	}
	if semanticExceptionAllowed(layer, stem) || validProductionStem(stem) {
		return Violation{}, false
	}
	if semanticException(stem) {
		detail := "文件名 " + stem + ".go 不是 " + layer + " 层允许的准确语义例外"
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: detail}, true
	}
	if invalidSubjectPrefix(stem) {
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: "业务主体必须是非泛化 snake_case 名称"}, true
	}
	return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: fileGrammarDetail}, true
}

func testFileNameViolation(file sourceFile, stem string) (Violation, bool) {
	for token := range strings.SplitSeq(stem, "_") {
		if slices.Contains([]string{"helpers", "utils", "common"}, token) {
			detail := "测试文件禁止泛化职责 token " + token + "；使用业务主体和可观察行为命名"
			return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: detail}, true
		}
	}
	return Violation{}, false
}

func forbiddenGenericFileViolation(file sourceFile, stem string) (Violation, bool) {
	forbiddenNames := []string{"models", "usecases", "results", "helpers", "utils", "common"}
	if slices.Contains(forbiddenNames, stem) {
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: "模块源码禁止泛化文件名 " + stem + ".go" + fileNameCorrectionDetail}, true
	}
	return Violation{}, false
}

func semanticExceptionAllowed(layer, stem string) bool {
	switch layer {
	case "root":
		return slices.Contains([]string{"api", "module", "authorization", "application_error"}, stem)
	case "domain":
		return slices.Contains([]string{"article", "article_type", "tag", "tag_article", "clock", "status", "errors"}, stem)
	case "postgres":
		return slices.Contains([]string{"unit_of_work", "error_translator", "model_time_mapper"}, stem)
	case "adapter":
		return stem == "postgres"
	default:
		return false
	}
}

func semanticException(stem string) bool {
	for _, layer := range []string{"root", "domain", "postgres", "adapter"} {
		if semanticExceptionAllowed(layer, stem) {
			return true
		}
	}
	return false
}

func validProductionStem(stem string) bool {
	for _, suffix := range productionSuffixes() {
		subject, found := strings.CutSuffix(stem, "_"+suffix)
		if found && validSubject(subject) {
			return true
		}
	}
	return false
}

func productionSuffixes() []string {
	return []string{
		"command_usecase_patch", "command_usecase", "query_usecase", "response_validator",
		"repository_port", "read_model_port", "result_mapper", "result_sort", "command_parser", "query_parser",
		"read_model", "command", "query", "result", "usecase", "port", "view", "model", "repository",
		"mapper", "validation", "error", "reconstitute", "rule", "handler", "etag", "pagination",
		"response", "translator", "sort", "parser", "validator", "value",
	}
}

func validSubject(subject string) bool {
	if subject == "" || strings.HasPrefix(subject, "_") || strings.HasSuffix(subject, "_") {
		return false
	}
	for token := range strings.SplitSeq(subject, "_") {
		if token == "" || invalidSubjectToken(token) || productionRoleToken(token) {
			return false
		}
		for _, character := range token {
			if character < 'a' || character > 'z' {
				return false
			}
		}
	}
	return true
}

func invalidSubjectPrefix(stem string) bool {
	first, _, _ := strings.Cut(stem, "_")
	return invalidSubjectToken(first)
}

func invalidSubjectToken(token string) bool {
	return slices.Contains([]string{"common", "shared", "utils", "utility", "helpers", "models", "usecases", "results"}, token)
}

func productionRoleToken(token string) bool {
	return slices.Contains([]string{
		"command", "query", "result", "usecase", "port", "view", "model", "repository", "read", "mapper",
		"validation", "error", "reconstitute", "rule", "handler", "etag", "pagination", "response", "translator",
		"sort", "parser", "validator", "value", "patch",
	}, token)
}
