package architecture

import (
	"go/ast"
	"go/token"
	"path"
	"strings"
)

func checkDeclarations(file sourceFile) []Violation {
	if file.relative == "internal/generated/openapi/openapi.gen.go" {
		return nil
	}
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
	if declaration.Tok == token.VAR && !isAllowedPackageVariable(file, declaration) {
		violations = append(violations, Violation{Code: "ARCH_MUTABLE_GLOBAL", Path: file.relative, Detail: "module package variables are mutable singletons; use const or constructor-owned state"})
	}
	if declaration.Tok != token.TYPE {
		return violations
	}
	for _, spec := range declaration.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		interfaceType, isInterface := typeSpec.Type.(*ast.InterfaceType)
		layer, _ := moduleLayer(file.relative)
		if isInterface && layer == "root" && interfaceType.Methods.NumFields() >= 2 {
			violations = append(violations, Violation{Code: "ARCH_UNIVERSAL_API", Path: file.relative, Detail: "universal API interface " + typeSpec.Name.Name + " is forbidden; transports own narrow interfaces"})
		}
		if isInterface && (typeSpec.TypeParams != nil || hasUniversalCRUD(interfaceType)) {
			violations = append(violations, Violation{Code: "ARCH_GENERIC_ABSTRACTION", Path: file.relative, Detail: "generic repository/service interface " + typeSpec.Name.Name + " is forbidden"})
		}
	}
	return violations
}

func isAllowedPackageVariable(file sourceFile, declaration *ast.GenDecl) bool {
	for _, spec := range declaration.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) != 1 {
			return false
		}
		if valueSpec.Names[0].Name == "_" {
			continue
		}
		comments := valueSpec.Doc
		if len(declaration.Specs) == 1 && comments == nil {
			comments = declaration.Doc
		}
		if isValidEmbed(file, valueSpec, comments) {
			continue
		}
		if len(valueSpec.Values) != 1 || !isStaticSentinel(file, valueSpec.Values[0]) {
			return false
		}
	}
	return true
}

func isStaticSentinel(file sourceFile, expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "New" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok || importedPath(file, packageName.Name) != "errors" {
		return false
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	return ok && literal.Kind == token.STRING
}

func isValidEmbed(file sourceFile, spec *ast.ValueSpec, comments *ast.CommentGroup) bool {
	if comments == nil || len(spec.Values) != 0 || !hasExactEmbedDirective(comments) {
		return false
	}
	if importedPath(file, "embed") != "embed" && importedPath(file, "_") != "embed" {
		return false
	}
	return isEmbedType(file, spec.Type)
}

func hasExactEmbedDirective(comments *ast.CommentGroup) bool {
	found := false
	for _, comment := range comments.List {
		if strings.HasPrefix(comment.Text, "//go:embed ") && len(strings.TrimSpace(strings.TrimPrefix(comment.Text, "//go:embed "))) > 0 {
			found = true
		}
	}
	return found
}

func isEmbedType(file sourceFile, expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name == "string"
	case *ast.ArrayType:
		element, ok := typed.Elt.(*ast.Ident)
		return typed.Len == nil && ok && element.Name == "byte"
	case *ast.SelectorExpr:
		packageName, ok := typed.X.(*ast.Ident)
		return ok && typed.Sel.Name == "FS" && importedPath(file, packageName.Name) == "embed"
	default:
		return false
	}
}

func importedPath(file sourceFile, localName string) string {
	for _, spec := range file.parsed.Imports {
		pathValue := strings.Trim(spec.Path.Value, `"`)
		name := path.Base(pathValue)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == localName && name != "." {
			return pathValue
		}
	}
	return ""
}

func hasUniversalCRUD(interfaceType *ast.InterfaceType) bool {
	operations := make(map[string]bool)
	for _, method := range interfaceType.Methods.List {
		for _, name := range method.Names {
			operation := strings.ToLower(name.Name)
			switch operation {
			case "create", "save", "get", "find", "list", "update", "delete":
				operations[operation] = true
			}
		}
	}
	return len(operations) >= 3
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
		parts := strings.Split(file.relative, "/")
		if len(parts) < 4 || parts[0] != "internal" || parts[1] != "modules" {
			continue
		}
		module := parts[2]
		if modules[module] == nil {
			modules[module] = make(map[string]bool)
		}
		if len(parts) == 4 {
			modules[module][path.Base(file.relative)] = true
		}
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
