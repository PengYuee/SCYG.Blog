package architecture

import (
	"strconv"
	"strings"
)

func checkImports(file sourceFile) []Violation {
	layer, module := moduleLayer(file.relative)
	externalTest := file.isExternalTest()
	if !externalTest && isContentRootPath(file.relative) && !isSharedRootFile(file.relative) {
		return []Violation{{Code: "ARCH_LEGACY_FILE", Path: file.relative, Detail: "content root file is not shared"}}
	}
	violations := make([]Violation, 0)
	for _, spec := range file.parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if importsModuleInternal(importPath) && !strings.HasPrefix(file.relative, "internal/modules/"+importedModule(importPath)+"/") {
			violations = append(violations, Violation{Code: "ARCH_INTERNAL_IMPORT", Path: file.relative, Detail: "bootstrap and transports must not import " + importPath})
		}
		if !externalTest && isForbiddenImport(layer, file.relative, importPath) {
			violations = append(violations, Violation{Code: "ARCH_FORBIDDEN_IMPORT", Path: file.relative, Detail: layer + " must not import " + importPath})
		}
		if !externalTest && illegalDirection(layer, module, file.relative, importPath) {
			violations = append(violations, Violation{Code: "ARCH_DEPENDENCY_DIRECTION", Path: file.relative, Detail: layer + " has an outward dependency on " + importPath})
		}
	}
	return violations
}

func isForbiddenImport(layer, file, importPath string) bool {
	boundaryLayer := layer == "root" || layer == "feature" || layer == "application"
	if boundaryLayer && (importPath == "net/http" || isForbiddenBoundaryImport(importPath)) {
		// Feature repositories and application transaction owners may import GORM;
		// transport, root and feature/application services must not import it.
		if (layer == "feature" || layer == "application") && strings.HasPrefix(importPath, "gorm.io/") {
			return false
		}
		return true
	}
	if layer == "application" && isApplicationForbiddenImport(importPath) {
		return true
	}
	if isTransportPath(file) && (strings.HasPrefix(importPath, "gorm.io/") || strings.HasPrefix(importPath, modulePath+"/internal/platform/database")) {
		return true
	}
	return false
}

func isForbiddenBoundaryImport(importPath string) bool {
	for _, prefix := range [...]string{
		"github.com/gin-gonic/gin",
		"gorm.io/",
		"github.com/spf13/viper",
		"google.golang.org/grpc",
		"google.golang.org/protobuf",
		"github.com/gorilla/websocket",
		"github.com/segmentio/kafka-go",
		modulePath + "/internal/generated/",
	} {
		if strings.HasPrefix(importPath, prefix) {
			return true
		}
	}
	return strings.Contains(importPath, "/generated/") || strings.Contains(importPath, "/api/proto/")
}

func isApplicationForbiddenImport(importPath string) bool {
	return strings.HasPrefix(importPath, modulePath+"/internal/platform/database")
}

func isTransportPath(path string) bool {
	return strings.HasPrefix(path, "internal/transport/")
}

func importsModuleInternal(importPath string) bool {
	parts := strings.SplitN(importPath, modulePath+"/internal/modules/", 2)
	return len(parts) == 2 && strings.Contains(parts[1], "/internal/")
}

func importedModule(importPath string) string {
	remaining := strings.SplitN(importPath, "/internal/modules/", 2)
	if len(remaining) != 2 {
		return ""
	}
	return strings.SplitN(remaining[1], "/", 2)[0]
}

func illegalDirection(layer, module, file, importPath string) bool {
	if module == "" {
		return false
	}
	base := modulePath + "/internal/modules/" + module
	if imported := importedModule(importPath); imported != "" && imported != module && (layer == "root" || layer == "feature" || layer == "application") {
		return true
	}
	if layer == "feature" {
		if packagePath(importPath, base+"/application") {
			return true
		}
		feature := featureFromFile(file)
		for _, sibling := range []string{"article", "taxonomy", "image"} {
			if sibling != feature && packagePath(importPath, base+"/"+sibling) {
				return true
			}
		}
	}
	if layer == "root" && (packagePath(importPath, base+"/application") || packagePath(importPath, base+"/article") || packagePath(importPath, base+"/taxonomy") || packagePath(importPath, base+"/image") || strings.HasPrefix(importPath, base+"/internal/")) {
		return true
	}
	return false
}

func packagePath(importPath, packagePath string) bool {
	return importPath == packagePath || strings.HasPrefix(importPath, packagePath+"/")
}

func featureFromFile(file string) string {
	parts := strings.Split(file, "/")
	if len(parts) >= 5 {
		return parts[3]
	}
	return ""
}

func moduleLayer(path string) (string, string) {
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[0] != "internal" || parts[1] != "modules" {
		return "", ""
	}
	module := parts[2]
	if len(parts) == 4 {
		return "root", module
	}
	if parts[3] == "application" {
		return "application", module
	}
	return "feature", module
}

func isContentRootPath(path string) bool {
	if !strings.HasPrefix(path, "internal/modules/content/") {
		return false
	}
	remaining := strings.TrimPrefix(path, "internal/modules/content/")
	return !strings.Contains(remaining, "/") && strings.HasSuffix(remaining, ".go")
}

func isSharedRootFile(path string) bool {
	switch path {
	case "internal/modules/content/security.go", "internal/modules/content/clock.go", "internal/modules/content/authorizer_regression_test.go", "internal/modules/content/current_author_test.go":
		return true
	default:
		return false
	}
}
