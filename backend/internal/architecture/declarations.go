package architecture

import (
	"go/ast"
	"path"
	"strings"
)

func checkDeclarations(file sourceFile) []Violation {
	violations := make([]Violation, 0)
	for _, declaration := range file.parsed.Decls {
		switch typed := declaration.(type) {
		case *ast.GenDecl:
			violations = append(violations, checkGeneralDeclaration(file, typed)...)
		case *ast.FuncDecl:
			if typed.Name.Name == "init" {
				violations = append(violations, Violation{Code: "ARCH_INIT_SIDE_EFFECT", Path: file.relative, Detail: "init side effects are forbidden"})
			}
		}
	}
	return violations
}

func checkGeneralDeclaration(file sourceFile, declaration *ast.GenDecl) []Violation {
	violations := make([]Violation, 0)
	if declaration.Tok.String() == "var" && strings.HasPrefix(file.relative, "internal/modules/") {
		violations = append(violations, Violation{Code: "ARCH_MUTABLE_GLOBAL", Path: file.relative, Detail: "module package variables are mutable singletons; use const or constructor-owned state"})
	}
	if declaration.Tok.String() != "type" {
		return violations
	}
	for _, spec := range declaration.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		_, isInterface := typeSpec.Type.(*ast.InterfaceType)
		if isInterface && typeSpec.Name.Name == "ContentAPI" {
			violations = append(violations, Violation{Code: "ARCH_UNIVERSAL_API", Path: file.relative, Detail: "universal API interface " + typeSpec.Name.Name + " is forbidden; transports own narrow interfaces"})
		}
		if isInterface && typeSpec.TypeParams != nil && (strings.Contains(typeSpec.Name.Name, "Repository") || strings.Contains(typeSpec.Name.Name, "Service")) {
			violations = append(violations, Violation{Code: "ARCH_GENERIC_ABSTRACTION", Path: file.relative, Detail: "generic repository/service interface " + typeSpec.Name.Name + " is forbidden"})
		}
	}
	return violations
}

func checkPath(file sourceFile) []Violation {
	if isForbiddenFuturePath(file.relative) {
		return []Violation{{Code: "ARCH_FORBIDDEN_FUTURE", Path: file.relative, Detail: "future module or adapter placeholders are forbidden"}}
	}
	if !strings.HasPrefix(file.relative, "internal/modules/") {
		return nil
	}
	for segment := range strings.SplitSeq(path.Dir(file.relative), "/") {
		switch segment {
		case "common", "shared", "utils", "utility", "helpers", "contract":
			return []Violation{{Code: "ARCH_FORBIDDEN_PACKAGE", Path: file.relative, Detail: "generic grab-bag or contract package " + segment + " is forbidden"}}
		}
	}
	return nil
}

func isForbiddenFuturePath(filePath string) bool {
	for _, prefix := range []string{
		"internal/integration/",
		"internal/modules/identity/",
		"internal/modules/media/",
		"internal/modules/search/",
		"internal/modules/ai/",
		"internal/transport/grpc/",
		"internal/transport/websocket/",
	} {
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	return false
}

func checkModuleShape(files []sourceFile) []Violation {
	modules := make(map[string]map[string]bool)
	for _, file := range files {
		_, module := moduleLayer(file.relative)
		if module == "" {
			continue
		}
		if modules[module] == nil {
			modules[module] = make(map[string]bool)
		}
		modules[module][path.Base(file.relative)] = true
	}
	violations := make([]Violation, 0)
	for module, names := range modules {
		for _, required := range []string{"api.go", "module.go"} {
			if !names[required] {
				violations = append(violations, Violation{Code: "ARCH_MODULE_SHAPE", Path: "internal/modules/" + module, Detail: "module root requires " + required})
			}
		}
	}
	return violations
}
