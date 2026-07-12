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
)

func checkFileOrganization(file sourceFile) []Violation {
	parts := strings.Split(file.relative, "/")
	if len(parts) < 4 || parts[0] != "internal" || parts[1] != "modules" {
		return nil
	}
	if violation, found := moduleLayerSubpackageViolation(file, parts); found {
		return []Violation{violation}
	}
	if violation, found := moduleFileNameViolation(file); found {
		return []Violation{violation}
	}
	return nil
}

func moduleLayerSubpackageViolation(file sourceFile, parts []string) (Violation, bool) {
	if len(parts) < 7 || parts[3] != "internal" || (parts[4] != "domain" && parts[4] != "application") {
		return Violation{}, false
	}
	subpackage := parts[5]
	detail := "domain/application 下禁止 Go 子 package " + subpackage + "；目录只表达技术层，业务主体必须使用文件名前缀"
	return Violation{Code: moduleLayerSubpackageCode, Path: file.relative, Detail: detail}, true
}

func moduleFileNameViolation(file sourceFile) (Violation, bool) {
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
	if allowedSemanticModuleFile(stem) || hasAllowedProductionRole(stem) {
		return Violation{}, false
	}
	return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: "模块源码文件必须使用 <subject>_<role>.go 或 <subject>_<role>_<subrole>.go"}, true
}

func testFileNameViolation(file sourceFile, stem string) (Violation, bool) {
	for token := range strings.SplitSeq(stem, "_") {
		if token == "helpers" || token == "utils" || token == "common" {
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

func allowedSemanticModuleFile(stem string) bool {
	allowedNames := []string{
		"api", "module", "clock", "status", "errors", "handler", "problem", "etag",
		"authorization", "application_error", "unit_of_work", "error_translator", "postgres",
		"article", "article_type", "tag", "tag_article", "taxonomy_pagination",
	}
	return slices.Contains(allowedNames, stem)
}

func hasAllowedProductionRole(stem string) bool {
	parts := strings.Split(stem, "_")
	if len(parts) < 2 {
		return false
	}
	for index := 1; index < len(parts); index++ {
		role := parts[index]
		if index+1 < len(parts) {
			role += "_" + parts[index+1]
		}
		if allowedProductionRole(parts[index]) || allowedProductionRole(role) {
			return true
		}
	}
	return false
}

func allowedProductionRole(role string) bool {
	switch role {
	case "command", "query", "result", "usecase", "port", "view", "model", "repository", "read_model", "mapper", "validation", "error", "reconstitute", "rule", "handler", "etag", "pagination", "response", "translator", "sort", "parser", "validator", "value":
		return true
	default:
		return false
	}
}
