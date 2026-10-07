package extractor

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	pgguard "github.com/iyiola-dev/pg-guard"
	"golang.org/x/tools/go/packages"
)

// sqlPkgFuncs maps receiver method names to whether they are context-aware.
var sqlPkgFuncs = map[string]bool{
	"Query":           false,
	"QueryRow":        false,
	"Exec":            false,
	"QueryContext":    true,
	"QueryRowContext": true,
	"ExecContext":     true,
}

var pgxFuncs = map[string]bool{
	"Query":    true,
	"QueryRow": true,
	"Exec":     true,
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
				hasCtx, sqlArgIndex, ok := queryCallInfo(pkg, sel)
				if !ok {
					return true
				}

				q := pgguard.ExtractedQuery{
					Pos:        fset.Position(call.Pos()),
					EndPos:     fset.Position(call.End()),
					FuncName:   funcName,
					HasContext: hasCtx,
					InLoop:     isInsideLoop(file, call, fset),
				}

				sqlArg := findSQLArg(call, sqlArgIndex)
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

// queryCallInfo resolves the declaring receiver so unrelated methods named
// Query or Exec cannot be mistaken for SQL calls. Using the method declaration
// also preserves calls promoted through an embedded database receiver.
func queryCallInfo(pkg *packages.Package, sel *ast.SelectorExpr) (hasContext bool, sqlArgIndex int, ok bool) {
	if pkg.TypesInfo == nil {
		return false, 0, false
	}
	selection := pkg.TypesInfo.Selections[sel]
	if selection == nil {
		return false, 0, false
	}
	signature, ok := selection.Obj().Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return false, 0, false
	}

	receiver := types.Unalias(signature.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, ok := receiver.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false, 0, false
	}

	name := named.Obj().Name()
	switch named.Obj().Pkg().Path() {
	case "database/sql":
		// Stmt methods take bound values, not SQL text; Prepare extraction is
		// outside this check, so do not interpret those values as a query.
		if name != "DB" && name != "Tx" && name != "Conn" {
			return false, 0, false
		}
		hasContext, ok = sqlPkgFuncs[sel.Sel.Name]
	case "github.com/jackc/pgx/v5":
		if name != "Conn" && name != "Tx" {
			return false, 0, false
		}
		hasContext, ok = pgxFuncs[sel.Sel.Name]
	case "github.com/jackc/pgx/v5/pgxpool":
		if name != "Pool" && name != "Conn" && name != "Tx" {
			return false, 0, false
		}
		hasContext, ok = pgxFuncs[sel.Sel.Name]
	case "gorm.io/gorm":
		if name != "DB" {
			return false, 0, false
		}
		hasContext, ok = gormFuncs[sel.Sel.Name]
	default:
		return false, 0, false
	}
	if !ok {
		return false, 0, false
	}
	if hasContext {
		sqlArgIndex = 1
	}
	if selection.Kind() == types.MethodExpr {
		// A method expression supplies its receiver as the first argument.
		sqlArgIndex++
	}
	return hasContext, sqlArgIndex, true
}

func findSQLArg(call *ast.CallExpr, idx int) ast.Expr {
	if idx < len(call.Args) {
		return call.Args[idx]
	}
	return nil
}

func classifySQL(expr ast.Expr) (sql, method string, parameterized bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		sql = strings.Trim(e.Value, `"`+"`")
		method = "literal"
		parameterized = strings.Contains(sql, "$1") || strings.Contains(sql, "?")
		return
	case *ast.CallExpr:
		if fn, ok := e.Fun.(*ast.SelectorExpr); ok {
			if fn.Sel.Name == "Sprintf" {
				if len(e.Args) > 0 {
					if lit, ok := e.Args[0].(*ast.BasicLit); ok {
						sql = strings.Trim(lit.Value, `"`+"`")
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
				sql = strings.Trim(lit.Value, `"`+"`")
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
