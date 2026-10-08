package architecture

import (
	"go/ast"
	"strconv"
)

func checkDeclarations(file sourceFile) []Violation {
	violations := make([]Violation, 0)
	generated := ast.IsGenerated(file.parsed)
	for _, declaration := range file.parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		// Generated protobuf registration is compiler output, not handwritten startup logic.
		if ok && function.Name.Name == "init" && !generated {
			violations = append(violations, Violation{Code: "ARCH_INIT_SIDE_EFFECT", Path: file.relative, Detail: "init side effects are forbidden"})
		}
	}
	violations = append(violations, checkApplicationSQL(file)...)
	return violations
}

// checkApplicationSQL permits transaction options without permitting SQL ownership.
func checkApplicationSQL(file sourceFile) []Violation {
	layer, _ := moduleLayer(file.relative)
	if layer != "application" || file.isExternalTest() {
		return nil
	}
	sqlName := ""
	for _, spec := range file.parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != "database/sql" {
			continue
		}
		sqlName = "sql"
		if spec.Name != nil {
			sqlName = spec.Name.Name
		}
	}
	violations := make([]Violation, 0)
	if sqlName == "." || sqlName == "_" {
		violations = append(violations, Violation{Code: "ARCH_APPLICATION_SQL", Path: file.relative, Detail: "database/sql must use a named import for transaction options"})
	}
	ast.Inspect(file.parsed, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok {
			if identifier, ok := selector.X.(*ast.Ident); ok && sqlName != "" && identifier.Name == sqlName && !isTransactionOption(selector.Sel.Name) {
				violations = append(violations, Violation{Code: "ARCH_APPLICATION_SQL", Path: file.relative, Detail: "application may use database/sql transaction options, not " + selector.Sel.Name})
			}
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				switch selector.Sel.Name {
				case "Raw", "Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext", "Prepare", "PrepareContext":
					violations = append(violations, Violation{Code: "ARCH_APPLICATION_SQL", Path: file.relative, Detail: "application must delegate direct SQL to feature repositories"})
				}
			}
		}
		return true
	})
	return violations
}

func isTransactionOption(name string) bool {
	switch name {
	case "TxOptions", "IsolationLevel", "LevelDefault", "LevelReadUncommitted", "LevelReadCommitted", "LevelWriteCommitted", "LevelRepeatableRead", "LevelSnapshot", "LevelSerializable", "LevelLinearizable":
		return true
	default:
		return false
	}
}
