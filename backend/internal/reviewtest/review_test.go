package reviewtest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

// repositoryRoot 返回包含 backend 的仓库根目录。
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 reviewtest 源文件")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func Test_ReviewE2E_AST_rejects_string_catalog(t *testing.T) {
	fixture, err := parser.ParseFile(token.NewFileSet(), "catalog_test.go", `package x; import "testing"; func Test_E2E_catalog(t *testing.T){ names:=[]string{"db"}; if len(names)!=1 { t.Fatal("x") } }`, 0)
	if err != nil {
		t.Fatalf("解析 E2E 失败夹具失败：%v", err)
	}
	function, ok := fixture.Decls[1].(*ast.FuncDecl)
	if !ok {
		t.Fatal("E2E 夹具声明类型错误")
	}
	if functionCalls(function, "newHarness") {
		t.Fatal("字符串清单被错误识别为真实 E2E")
	}
}

func Test_ReviewE2E_AST_rejects_missing_concrete_comparison(t *testing.T) {
	fixture, err := parser.ParseFile(token.NewFileSet(), "weak_test.go", `package x; import "testing"; func Test_E2E_weak(t *testing.T){ snapshotDatabase(); request(); if true { t.Fatal("任意条件") } }`, 0)
	if err != nil {
		t.Fatalf("解析弱断言夹具失败：%v", err)
	}
	function, ok := fixture.Decls[1].(*ast.FuncDecl)
	if !ok {
		t.Fatal("弱断言夹具声明类型错误")
	}
	symbols := functionSymbolSet(function)
	if symbols["DeepEqual"] || symbols["StatusForbidden"] {
		t.Fatal("弱断言夹具错误包含具体状态比较")
	}
}

func functionSymbolSet(function *ast.FuncDecl) map[string]bool {
	symbols := make(map[string]bool)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			symbols[callName(call.Fun)] = true
		}
		if selector, ok := node.(*ast.SelectorExpr); ok {
			symbols[selector.Sel.Name] = true
		}
		return true
	})
	return symbols
}

func functionCalls(function *ast.FuncDecl, name string) bool { return functionCallsAny(function, name) }

func functionCallsAny(function *ast.FuncDecl, names ...string) bool {
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, name := range names {
			if callName(call.Fun) == name {
				found = true
			}
		}
		return true
	})
	return found
}

func callName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}
