package architecture

import (
	"path"
	"slices"
	"strings"
)

const (
	moduleFileNameCode        = "ARCH_MODULE_FILE_NAME"
	moduleLayerSubpackageCode = "ARCH_MODULE_LAYER_SUBPACKAGE"
	productionNameDetail      = "模块源码文件名必须是非空、非泛化的小写 snake_case"
	testNameDetail            = "模块测试文件名必须是非空、非泛化的小写 snake_case"
	anchorLocationDetail      = "api.go 和 module.go 仅允许位于模块根目录"
	layerPackageDetail        = "模块 Go package 仅允许根、internal/domain、internal/application、internal/postgres 和 postgres；禁止额外子 package "
)

func checkFileOrganization(file sourceFile) []Violation {
	parts := strings.Split(file.relative, "/")
	if len(parts) < 2 || parts[0] != "internal" || parts[1] != "modules" {
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
	// internal/modules 下的 Go 文件必须先进入具体模块目录，不能绕过五层拓扑。
	if len(parts) < 3 {
		return "", "internal/modules"
	}
	if len(parts) < 4 {
		return "", parts[2]
	}
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
	if !validSnakeCase(stem) {
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: productionNameDetail}, true
	}
	if (stem == "api" || stem == "module") && layer != "root" {
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: anchorLocationDetail}, true
	}
	if token, found := genericToken(stem); found {
		detail := "模块源码文件名禁止泛化 token " + token + "；使用具体业务名称"
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: detail}, true
	}
	if strings.HasSuffix(stem, "_record") {
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: "数据库数据模型文件必须使用 <subject>_model.go，禁止 *_record.go"}, true
	}
	return Violation{}, false
}

func testFileNameViolation(file sourceFile, stem string) (Violation, bool) {
	if !validSnakeCase(stem) {
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: testNameDetail}, true
	}
	if token, found := genericToken(stem); found {
		detail := "模块测试文件名禁止泛化 token " + token + "；使用具体业务和可观察行为命名"
		return Violation{Code: moduleFileNameCode, Path: file.relative, Detail: detail}, true
	}
	return Violation{}, false
}

// validSnakeCase 只执行可长期稳定的机械校验；职责词会持续演进，因此不在 Scanner 中枚举。
func validSnakeCase(stem string) bool {
	if stem == "" {
		return false
	}
	for token := range strings.SplitSeq(stem, "_") {
		if token == "" {
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

// genericToken 对生产和测试文件复用同一组泛化 token，避免两套规则漂移。
func genericToken(stem string) (string, bool) {
	for token := range strings.SplitSeq(stem, "_") {
		if invalidSubjectToken(token) {
			return token, true
		}
	}
	return "", false
}

func invalidSubjectToken(token string) bool {
	return slices.Contains([]string{"common", "shared", "utils", "utility", "helpers", "models", "usecases", "results"}, token)
}
