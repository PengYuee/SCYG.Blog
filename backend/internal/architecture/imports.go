package architecture

import (
	"strconv"
	"strings"
)

func checkImports(file sourceFile) []Violation {
	violations := make([]Violation, 0)
	layer, module := moduleLayer(file.relative)
	for _, spec := range file.parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if importsModuleInternal(importPath) && !strings.HasPrefix(file.relative, "internal/modules/"+importedModule(importPath)+"/") {
			violations = append(violations, Violation{Code: "ARCH_INTERNAL_IMPORT", Path: file.relative, Detail: "bootstrap and transports must not import " + importPath})
		}
		if layer == "domain" || layer == "application" || layer == "root" {
			if isForbiddenBoundaryImport(importPath) || importPath == "net/http" {
				violations = append(violations, Violation{Code: "ARCH_FORBIDDEN_IMPORT", Path: file.relative, Detail: layer + " must not import " + importPath})
			}
		}
		if illegalDirection(layer, module, importPath) {
			violations = append(violations, Violation{Code: "ARCH_DEPENDENCY_DIRECTION", Path: file.relative, Detail: layer + " has an outward dependency on " + importPath})
		}
	}
	return violations
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

func importsModuleInternal(importPath string) bool {
	parts := strings.SplitN(importPath, "/internal/modules/", 2)
	return len(parts) == 2 && strings.Contains(parts[1], "/internal/")
}

func importedModule(importPath string) string {
	remaining := strings.SplitN(importPath, "/internal/modules/", 2)
	if len(remaining) != 2 {
		return ""
	}
	return strings.SplitN(remaining[1], "/", 2)[0]
}

func illegalDirection(layer, module, importPath string) bool {
	base := modulePath + "/internal/modules/" + module
	switch layer {
	case "domain":
		return strings.HasPrefix(importPath, base+"/internal/application") || strings.HasPrefix(importPath, base+"/internal/postgres")
	case "application":
		return strings.HasPrefix(importPath, base+"/internal/postgres")
	case "root":
		return strings.HasPrefix(importPath, base+"/internal/postgres")
	default:
		return false
	}
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
	if len(parts) >= 6 && parts[3] == "internal" {
		return parts[4], module
	}
	return "", module
}
