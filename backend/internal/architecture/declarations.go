package architecture

import (
	"go/ast"
)

func checkDeclarations(file sourceFile) []Violation {
	violations := make([]Violation, 0)
	for _, declaration := range file.parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "init" {
			violations = append(violations, Violation{Code: "ARCH_INIT_SIDE_EFFECT", Path: file.relative, Detail: "init side effects are forbidden"})
		}
	}
	return violations
}
