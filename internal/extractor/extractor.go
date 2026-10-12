package extractor

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/sqltext"
)

const (
	pkgSQL        = "database/sql"
	pkgGORM       = "gorm.io/gorm"
	pkgGORMClause = "gorm.io/gorm/clause"

	// maxDepth bounds how far SQL values and GORM chains are traced.
	maxDepth = 16
)

// pgxPkgs are the pgx packages whose connection, pool and transaction types
// share pgx's query methods.
var pgxPkgs = map[string]bool{
	"github.com/jackc/pgx/v5":         true,
	"github.com/jackc/pgx/v5/pgxpool": true,
	"github.com/jackc/pgx/v4":         true,
	"github.com/jackc/pgx/v4/pgxpool": true,
}

// sqlResultPkgs and pgxResultPkgs hold the result types that identify
// database/sql- and pgx-shaped methods on other types.
var (
	sqlResultPkgs = map[string]bool{pkgSQL: true}
	pgxResultPkgs = map[string]bool{
		"github.com/jackc/pgx/v5":        true,
		"github.com/jackc/pgx/v5/pgconn": true,
		"github.com/jackc/pgx/v4":        true,
		"github.com/jackc/pgconn":        true,
	}
)

// method says where a database/sql or pgx method takes its context and SQL.
type method struct {
	call   pgguard.CallKind
	ctx    bool // the first parameter is a context.Context
	sqlArg int  // index of the SQL parameter, or -1
}

var sqlMethods = map[string]method{
	"Query":           {pgguard.CallQuery, false, 0},
	"QueryRow":        {pgguard.CallQuery, false, 0},
	"Exec":            {pgguard.CallQuery, false, 0},
	"Prepare":         {pgguard.CallPrepare, false, 0},
	"QueryContext":    {pgguard.CallQuery, true, 1},
	"QueryRowContext": {pgguard.CallQuery, true, 1},
	"ExecContext":     {pgguard.CallQuery, true, 1},
	"PrepareContext":  {pgguard.CallPrepare, true, 1},
}

var pgxMethods = map[string]method{
	"Query":    {pgguard.CallQuery, true, 1},
	"QueryRow": {pgguard.CallQuery, true, 1},
	"Exec":     {pgguard.CallQuery, true, 1},
	"Prepare":  {pgguard.CallPrepare, true, 2},
}

// Extract returns the database calls in files, which must make up a single
// package described by info.
func Extract(fset *token.FileSet, files []*ast.File, info *types.Info) []pgguard.ExtractedQuery {
	x := &extractor{
		fset: fset,
		info: info,
	}
	for _, file := range files {
		var stack []ast.Node
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			if call, ok := n.(*ast.CallExpr); ok {
				x.call(call, stack)
			}
			return true
		})
	}
	return x.queries
}

type extractor struct {
	fset    *token.FileSet
	info    *types.Info
	queries []pgguard.ExtractedQuery
}

func (x *extractor) call(call *ast.CallExpr, stack []ast.Node) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return
	}
	fn, ok := x.info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return
	}
	q, ok := x.classify(call, fn)
	if !ok {
		return
	}
	// Report at the method name so that each call in a chain has its own
	// position.
	q.Pos = x.fset.Position(sel.Sel.Pos())
	q.TokenPos = sel.Sel.Pos()
	q.FuncName = fn.Name()
	q.InLoop = inLoop(stack)
	x.queries = append(x.queries, q)
}

// classify recognizes calls by the type that declares the method, so that
// same-named methods of other libraries are not mistaken for queries.
func (x *extractor) classify(call *ast.CallExpr, fn *types.Func) (pgguard.ExtractedQuery, bool) {
	sig := fn.Type().(*types.Signature)
	pkg, recv := fn.Pkg().Path(), ""
	if sig.Recv() != nil {
		recv = typeName(sig.Recv().Type())
	}

	switch {
	case pkg == pkgSQL && (recv == "DB" || recv == "Tx" || recv == "Conn"):
		if m, ok := sqlMethods[fn.Name()]; ok {
			return x.direct(call, pgguard.APIDatabaseSQL, m), true
		}
	case pkg == pkgSQL && recv == "Stmt":
		// Arguments to a prepared statement are values; its SQL is checked
		// where it is prepared.
		if m, ok := sqlMethods[fn.Name()]; ok && m.call == pgguard.CallQuery {
			m.sqlArg = -1
			return x.direct(call, pgguard.APIDatabaseSQL, m), true
		}
	case pgxPkgs[pkg] && recv == "Batch":
		if fn.Name() == "Queue" {
			return x.direct(call, pgguard.APIPgx, method{call: pgguard.CallBuild, ctx: true}), true
		}
	case pgxPkgs[pkg] && sig.Recv() != nil:
		if m, ok := pgxMethods[fn.Name()]; ok && matches(sig, m) {
			return x.direct(call, pgguard.APIPgx, m), true
		}
	case sig.Recv() != nil:
		return x.structural(call, fn.Name(), sig)
	}
	return pgguard.ExtractedQuery{}, false
}

// structural recognizes methods shaped like database/sql's or pgx's query
// methods on other types, such as the DBTX interfaces sqlc generates.
func (x *extractor) structural(call *ast.CallExpr, name string, sig *types.Signature) (pgguard.ExtractedQuery, bool) {
	if m, ok := sqlMethods[name]; ok && matches(sig, m) && returnsFrom(sig, sqlResultPkgs) {
		return x.direct(call, pgguard.APIDatabaseSQL, m), true
	}
	if m, ok := pgxMethods[name]; ok && matches(sig, m) && returnsFrom(sig, pgxResultPkgs) {
		return x.direct(call, pgguard.APIPgx, m), true
	}
	return pgguard.ExtractedQuery{}, false
}

// direct describes a database/sql or pgx call, whose SQL is a complete
// statement.
func (x *extractor) direct(call *ast.CallExpr, api pgguard.API, m method) pgguard.ExtractedQuery {
	q := pgguard.ExtractedQuery{API: api, Call: m.call, HasContext: m.ctx}
	if m.sqlArg < 0 || m.sqlArg >= len(call.Args) {
		return q
	}
	var p sqltext.Placeholders
	if len(call.Args) > m.sqlArg+1 && x.isNamedArgs(call.Args[m.sqlArg+1]) {
		p.Named = true
	}
	setSQL(&q, x.eval(call.Args[m.sqlArg], 0), pgguard.SQLStatement, "", p, true)
	return q
}

func (x *extractor) isNamedArgs(e ast.Expr) bool {
	t, ok := types.Unalias(x.info.TypeOf(e)).(*types.Named)
	if !ok || t.Obj().Pkg() == nil || !pgxPkgs[t.Obj().Pkg().Path()] {
		return false
	}
	return t.Obj().Name() == "NamedArgs" || t.Obj().Name() == "StrictNamedArgs"
}

func setSQL(q *pgguard.ExtractedQuery, v value, kind pgguard.SQLKind, clause string, p sqltext.Placeholders, exact bool) {
	q.Kind, q.Clause, q.SQL, q.BuildMethod = kind, clause, v.text, v.method
	if v.exact && exact {
		q.NormalizedSQL = sqltext.Rewrite(v.text, p)
	}
}

// value is what is statically known about a SQL string.
type value struct {
	text   string // the SQL when exact, otherwise a template or ""
	method pgguard.BuildMethod
	exact  bool
}

var unknown = value{method: pgguard.BuildUnknown}

func (x *extractor) eval(e ast.Expr, depth int) value {
	if depth > maxDepth {
		return unknown
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			text, err := strconv.Unquote(e.Value)
			if err == nil {
				return value{text: text, method: pgguard.BuildConstant, exact: true}
			}
		}
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			return x.evalConcat(e, depth)
		}
	case *ast.CallExpr:
		if x.isFunc(e, "fmt", "Sprintf") {
			return x.evalSprintf(e, depth)
		}
	}
	return unknown
}

func (x *extractor) evalConcat(e *ast.BinaryExpr, depth int) value {
	l, r := x.eval(e.X, depth+1), x.eval(e.Y, depth+1)
	return value{text: template(e.X, l) + template(e.Y, r), method: pgguard.BuildConcat}
}

// template renders v for display, showing non-constant parts as {expr}.
func template(e ast.Expr, v value) string {
	if v.exact || v.text != "" {
		return v.text
	}
	return "{" + types.ExprString(e) + "}"
}

func (x *extractor) evalSprintf(call *ast.CallExpr, depth int) value {
	v := value{method: pgguard.BuildSprintf}
	if len(call.Args) > 0 {
		v.text = x.eval(call.Args[0], depth+1).text
	}
	return v
}

func (x *extractor) isFunc(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := x.info.Uses[sel.Sel].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == pkg && fn.Name() == name &&
		fn.Type().(*types.Signature).Recv() == nil
}

// inLoop reports whether the innermost node of stack runs once per
// iteration of an enclosing loop in the same function.
func inLoop(stack []ast.Node) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		child := stack[i+1]
		switch n := stack[i].(type) {
		case *ast.ForStmt:
			if child == n.Body || child == n.Cond || child == n.Post {
				return true
			}
		case *ast.RangeStmt:
			if child == n.Body {
				return true
			}
		case *ast.FuncDecl:
			return false
		}
	}
	return false
}

// matches reports whether sig takes a context and an SQL string where m
// expects them.
func matches(sig *types.Signature, m method) bool {
	params := sig.Params()
	if m.sqlArg >= params.Len() || !isString(params.At(m.sqlArg).Type()) {
		return false
	}
	return !m.ctx || isContext(params.At(0).Type())
}

func returnsFrom(sig *types.Signature, pkgs map[string]bool) bool {
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		if pkgs[typePkg(results.At(i).Type())] {
			return true
		}
	}
	return false
}

func isContext(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "context" && n.Obj().Name() == "Context"
}

func isString(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

func isPackageLevel(v *types.Var) bool {
	return v.Pkg() != nil && v.Pkg().Scope().Lookup(v.Name()) == v
}

// typeName returns the name of the named type t or *t.
func typeName(t types.Type) string {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := types.Unalias(t).(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

// typePkg returns the import path of the package declaring the named type
// t or *t.
func typePkg(t types.Type) string {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil {
		return n.Obj().Pkg().Path()
	}
	return ""
}
