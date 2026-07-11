package reviewtest_test

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

// forbiddenModulePrefixes 返回本阶段禁止进入模块图或生产导入的前缀。
func forbiddenModulePrefixes() []string {
	return []string{"google.golang.org/grpc", "github.com/gorilla/websocket", "github.com/segmentio/kafka-go", "buf.build/"}
}

// isForbiddenDirectory 按完整路径段识别未来运行时目录。
func isForbiddenDirectory(name string) bool {
	switch name {
	case "grpc", "websocket", "kafka", "outbox", "proto", "integration":
		return true
	default:
		return false
	}
}
func forbiddenRuntimeDirectory(path string) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(path)), "/")
	for index, part := range parts {
		if isForbiddenDirectory(part) && !(part == "integration" && index > 0 && parts[index-1] == "testdata") {
			return true
		}
	}
	return false
}

func scanModule(path string) ([]scopeViolation, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, err := modfile.Parse(path, content, nil)
	if err != nil {
		return nil, err
	}
	violations := make([]scopeViolation, 0)
	for _, requirement := range file.Require {
		for _, prefix := range forbiddenModulePrefixes() {
			if strings.HasPrefix(requirement.Mod.Path, prefix) {
				violations = append(violations, scopeViolation{"go.mod", "禁止模块依赖：" + requirement.Mod.Path})
			}
		}
	}
	return violations, nil
}

func scanGoAST(path string, file *ast.File) []scopeViolation {
	violations := make([]scopeViolation, 0)
	constants := collectStringConstants(file)
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		for _, prefix := range forbiddenModulePrefixes() {
			if strings.HasPrefix(value, prefix) {
				violations = append(violations, scopeViolation{path, "禁止运行时导入：" + value})
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncDecl:
			if isAllowAllName(value.Name.Name) || authorizeReturnsNil(value) {
				violations = append(violations, scopeViolation{path, "禁止生产放行授权实现"})
			}
		case *ast.TypeSpec:
			if isAllowAllName(value.Name.Name) {
				violations = append(violations, scopeViolation{path, "AllowAll 只能存在于测试源码"})
			}
			if value.Name.Name == "Category" {
				violations = append(violations, scopeViolation{path, "实体 ArticleType 禁止改名为 Category"})
			}
		case *ast.CallExpr:
			if selector, ok := value.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "AutoMigrate" {
				violations = append(violations, scopeViolation{path, "禁止调用 AutoMigrate"})
			}
			for _, argument := range value.Args {
				if route, ok := constantString(argument, constants); ok && legacyRoute(route) {
					violations = append(violations, scopeViolation{path, "禁止旧式 action 路由：" + route})
				}
			}
		case *ast.ValueSpec:
			for _, name := range value.Names {
				if isAllowAllName(name.Name) {
					violations = append(violations, scopeViolation{path, "禁止生产 AllowAll 变量"})
				}
			}
			for _, expression := range value.Values {
				if route, ok := constantString(expression, constants); ok && legacyRoute(route) {
					violations = append(violations, scopeViolation{path, "禁止旧式 action 路由：" + route})
				}
			}
		}
		return true
	})
	return violations
}

// isAllowAllName 归一化大小写和下划线后识别测试放行策略名。
func isAllowAllName(name string) bool {
	normalized := strings.ReplaceAll(strings.ToLower(name), "_", "")
	return strings.Contains(normalized, "allowall")
}

// authorizeReturnsNil 识别无条件返回 nil 的生产 Authorize 方法。
func authorizeReturnsNil(function *ast.FuncDecl) bool {
	if function.Name.Name != "Authorize" || function.Body == nil || len(function.Body.List) != 1 {
		return false
	}
	result, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(result.Results) != 1 {
		return false
	}
	identifier, ok := result.Results[0].(*ast.Ident)
	return ok && identifier.Name == "nil"
}

// collectStringConstants 迭代解析包级字符串常量，支持标识符和括号引用。
func collectStringConstants(file *ast.File) map[string]string {
	constants := make(map[string]string)
	for changed := true; changed; {
		changed = false
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, specification := range general.Specs {
				value := specification.(*ast.ValueSpec)
				for index, name := range value.Names {
					if index >= len(value.Values) {
						continue
					}
					if result, ok := constantString(value.Values[index], constants); ok && constants[name.Name] != result {
						constants[name.Name] = result
						changed = true
					}
				}
			}
		}
	}
	return constants
}

// constantString 折叠字符串字面量拼接，避免通过空白或 `+` 绕过旧路由扫描。
func constantString(expression ast.Expr, constants map[string]string) (string, bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		result, err := strconv.Unquote(value.Value)
		return result, err == nil
	case *ast.ParenExpr:
		return constantString(value.X, constants)
	case *ast.Ident:
		result, ok := constants[value.Name]
		return result, ok
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := constantString(value.X, constants)
		right, rightOK := constantString(value.Y, constants)
		return left + right, leftOK && rightOK
	default:
		return "", false
	}
}

func legacyRoute(route string) bool {
	lower := strings.ToLower(route)
	return strings.Contains(lower, "/article/get") || strings.Contains(lower, "/article/create") || strings.Contains(lower, "/article/update") || strings.Contains(lower, "/article/delete")
}
