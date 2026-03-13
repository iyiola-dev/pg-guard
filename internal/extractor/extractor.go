package extractor

import (
	"go/ast"
	"go/token"
	"strings"

	pgguard "github.com/iyiola-dev/pg-guard"
	"golang.org/x/tools/go/packages"
)

// sqlPkgFuncs maps receiver method names to whether they are context-aware.
var sqlPkgFuncs = map[string]bool{
	"Query":          false,
	"QueryRow":       false,
	"Exec":           false,
	"QueryContext":   true,
	"QueryRowContext": true,
	"ExecContext":    true,
}

var pgxFuncs = map[string]bool{
	"Query":   true,
	"Exec":    true,
	"SendBatch": true,
}

var gormFuncs = map[string]bool{
	"Raw":   false,
	"Exec":  false,
	"Where": false,
}

func Extract(pkgs []*packages.Package) []pgguard.ExtractedQuery {
	var queries []pgguard.ExtractedQuery

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			fset := pkg.Fset
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				funcName := sel.Sel.Name
				hasCtx, isSQL := sqlPkgFuncs[funcName]
				_, isPgx := pgxFuncs[funcName]
				_, isGorm := gormFuncs[funcName]

				if !isSQL && !isPgx && !isGorm {
					return true
				}

				q := pgguard.ExtractedQuery{
					Pos:       fset.Position(call.Pos()),
					EndPos:    fset.Position(call.End()),
					FuncName:  funcName,
					HasContext: hasCtx || isPgx,
					InLoop:    isInsideLoop(file, call, fset),
				}

				sqlArg := findSQLArg(call, hasCtx)
				if sqlArg != nil {
					q.SQL, q.BuildMethod, q.Parameterized = classifySQL(sqlArg)
				} else {
					q.BuildMethod = "unknown"
				}

				queries = append(queries, q)
				return true
			})
		}
	}

	return queries
}

func findSQLArg(call *ast.CallExpr, hasContext bool) ast.Expr {
	idx := 0
	if hasContext && len(call.Args) > 1 {
		idx = 1
	}
	if idx < len(call.Args) {
		return call.Args[idx]
	}
	return nil
}

func classifySQL(expr ast.Expr) (sql, method string, parameterized bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		sql = strings.Trim(e.Value, `"` + "`")
		method = "literal"
		parameterized = strings.Contains(sql, "$1") || strings.Contains(sql, "?")
		return
	case *ast.CallExpr:
		if fn, ok := e.Fun.(*ast.SelectorExpr); ok {
			if fn.Sel.Name == "Sprintf" {
				if len(e.Args) > 0 {
					if lit, ok := e.Args[0].(*ast.BasicLit); ok {
						sql = strings.Trim(lit.Value, `"` + "`")
					}
				}
				method = "sprintf"
				parameterized = false
				return
			}
		}
		method = "unknown"
		return
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			method = "concat"
			parameterized = false
			if lit, ok := e.X.(*ast.BasicLit); ok {
				sql = strings.Trim(lit.Value, `"` + "`")
			}
			return
		}
		method = "unknown"
		return
	default:
		method = "unknown"
		return
	}
}

func isInsideLoop(file *ast.File, target ast.Node, fset *token.FileSet) bool {
	found := false
	var walk func(ast.Node) bool
	walk = func(n ast.Node) bool {
		if found {
			return false
		}
		if n == target {
			return false
		}
		switch n.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			ast.Inspect(n, func(inner ast.Node) bool {
				if inner == target {
					found = true
					return false
				}
				return true
			})
			if found {
				return false
			}
		}
		return true
	}
	ast.Inspect(file, walk)
	return found
}
