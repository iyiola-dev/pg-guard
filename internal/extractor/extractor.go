package extractor

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

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

// gormMethod says what a *gorm.DB method does with its arguments.
type gormMethod struct {
	call   pgguard.CallKind
	sqlArg int // index of the SQL argument when kind is not SQLNone
	kind   pgguard.SQLKind
	clause string
	cond   bool // the SQL argument is a condition, interpreted as Where does
}

var (
	gormCond       = gormMethod{call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseWhere, cond: true}
	gormInlineCond = gormMethod{call: pgguard.CallQuery, sqlArg: 1, kind: pgguard.SQLFragment, clause: sqltext.ClauseWhere, cond: true}
	gormJoin       = gormMethod{call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseJoin}
	gormFinisher   = gormMethod{call: pgguard.CallQuery}
)

var gormMethods = map[string]gormMethod{
	"Raw":    {call: pgguard.CallBuild, kind: pgguard.SQLStatement},
	"Exec":   {call: pgguard.CallQuery, kind: pgguard.SQLStatement},
	"Where":  gormCond,
	"Or":     gormCond,
	"Not":    gormCond,
	"Having": {call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseHaving, cond: true},
	"Select": {call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseSelect},
	"Order":  {call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseOrderBy},
	"Group":  {call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseGroupBy},
	"Table":  {call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseFrom},
	"Pluck":  {call: pgguard.CallQuery, kind: pgguard.SQLFragment, clause: sqltext.ClauseSelect},

	"Joins":      gormJoin,
	"InnerJoins": gormJoin,

	"First":         gormInlineCond,
	"Take":          gormInlineCond,
	"Last":          gormInlineCond,
	"Find":          gormInlineCond,
	"FirstOrInit":   gormInlineCond,
	"FirstOrCreate": gormInlineCond,
	"Delete":        gormInlineCond,

	"Scan":            gormFinisher,
	"Row":             gormFinisher,
	"Rows":            gormFinisher,
	"Count":           gormFinisher,
	"Create":          gormFinisher,
	"CreateInBatches": gormFinisher,
	"Save":            gormFinisher,
	"Update":          gormFinisher,
	"Updates":         gormFinisher,
	"UpdateColumn":    gormFinisher,
	"UpdateColumns":   gormFinisher,
	"FindInBatches":   gormFinisher,
}

// Extract returns the database calls in files, which must make up a single
// package described by info.
func Extract(fset *token.FileSet, files []*ast.File, info *types.Info) []pgguard.ExtractedQuery {
	x := &extractor{
		fset:     fset,
		info:     info,
		assigns:  indexAssignments(files, info),
		visiting: make(map[*types.Var]bool),
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
	fset     *token.FileSet
	info     *types.Info
	assigns  map[*types.Var]*assignments
	visiting map[*types.Var]bool
	queries  []pgguard.ExtractedQuery
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
	case pkg == pkgGORM && recv == "DB":
		return x.gorm(call, fn.Name())
	case pkg == pkgGORM && sig.Recv() == nil && fn.Name() == "Expr":
		return x.gormSQL(call, gormMethod{call: pgguard.CallBuild, kind: pgguard.SQLFragment, clause: sqltext.ClauseSelect})
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

func (x *extractor) gorm(call *ast.CallExpr, name string) (pgguard.ExtractedQuery, bool) {
	m, ok := gormMethods[name]
	if !ok {
		return pgguard.ExtractedQuery{}, false
	}
	q, hasSQL := x.gormSQL(call, m)
	if m.call != pgguard.CallQuery {
		return q, hasSQL
	}
	q.HasContext = x.gormContext(ast.Unparen(call.Fun).(*ast.SelectorExpr).X)
	return q, true
}

// gormSQL describes the SQL text a GORM call takes, reporting false when
// the call takes none.
func (x *extractor) gormSQL(call *ast.CallExpr, m gormMethod) (pgguard.ExtractedQuery, bool) {
	q := pgguard.ExtractedQuery{API: pgguard.APIGORM, Call: m.call, HasContext: true}
	if m.kind == pgguard.SQLNone || m.sqlArg >= len(call.Args) {
		return q, false
	}
	arg := call.Args[m.sqlArg]
	if !isString(x.info.TypeOf(arg)) {
		return q, false // e.g. Where(&User{Name: name}) or Order(clause.OrderByColumn{...})
	}

	vars := call.Args[m.sqlArg+1:]
	bound := len(vars)
	if call.Ellipsis.IsValid() {
		bound = -1
	}
	v := x.eval(arg, 0)
	if v.exact && !gormIsSQL(v.text, m, bound) {
		return q, false
	}
	p := sqltext.Placeholders{Question: bound, Named: bound != 0 && strings.Contains(v.text, "@")}
	setSQL(&q, v, m.kind, m.clause, p, !x.expandsToSQL(vars))
	return q, true
}

// gormIsSQL reports whether GORM reads text as SQL rather than as a primary
// key, column name or association name.
func gormIsSQL(text string, m gormMethod, bound int) bool {
	trimmed := strings.TrimSpace(text)
	switch {
	case trimmed == "":
		return false
	case m.cond:
		if _, err := strconv.Atoi(text); err == nil {
			return false // a primary key
		}
		return bound == 0 || strings.ContainsAny(text, "?@") || strings.ContainsAny(trimmed, " \t\n")
	case m.clause == sqltext.ClauseJoin:
		return strings.ContainsAny(trimmed, " \t\n") // otherwise an association name
	}
	return true
}

// expandsToSQL reports whether any argument is spliced into the SQL as text
// rather than bound as a value, as GORM does with subqueries and clause
// expressions.
func (x *extractor) expandsToSQL(args []ast.Expr) bool {
	for _, a := range args {
		if path := typePkg(x.info.TypeOf(a)); path == pkgGORM || path == pkgGORMClause {
			return true
		}
	}
	return false
}

// gormContext reports whether the GORM chain e carries a context. A chain
// whose origin cannot be traced, such as a *gorm.DB parameter or the tx of a
// Transaction callback, counts as carrying one, so only definite omissions
// are reported.
func (x *extractor) gormContext(e ast.Expr) bool {
	for range maxDepth {
		switch n := ast.Unparen(e).(type) {
		case *ast.CallExpr:
			sel, ok := ast.Unparen(n.Fun).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := x.info.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != pkgGORM {
				return true
			}
			switch fn.Name() {
			case "WithContext":
				return true
			case "Open":
				return false
			case "Session":
				if hasCtx, known := sessionContext(n); hasCtx || !known {
					return true
				}
			}
			if fn.Type().(*types.Signature).Recv() == nil {
				return true
			}
			e = sel.X
		case *ast.SelectorExpr:
			// Struct fields and package variables hold long-lived handles.
			v, ok := x.info.Uses[n.Sel].(*types.Var)
			return !ok || !(v.IsField() || isPackageLevel(v))
		case *ast.Ident:
			v, ok := x.info.Uses[n].(*types.Var)
			if !ok {
				return true
			}
			if isPackageLevel(v) {
				return false
			}
			as := x.assigns[v]
			if as == nil || as.escaped || len(as.values) != 1 || as.values[0].expr == nil || as.values[0].append {
				return true
			}
			e = as.values[0].expr
		default:
			return true
		}
	}
	return true
}

// sessionContext reports whether a Session(&gorm.Session{...}) call sets a
// Context, and whether that could be determined.
func sessionContext(call *ast.CallExpr) (hasCtx, known bool) {
	if len(call.Args) != 1 {
		return false, false
	}
	arg := ast.Unparen(call.Args[0])
	if u, ok := arg.(*ast.UnaryExpr); ok && u.Op == token.AND {
		arg = ast.Unparen(u.X)
	}
	lit, ok := arg.(*ast.CompositeLit)
	if !ok {
		return false, false
	}
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Context" {
				return true, true
			}
		}
	}
	return false, true
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
	e = ast.Unparen(e)
	if tv, ok := x.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return value{text: constant.StringVal(tv.Value), method: pgguard.BuildConstant, exact: true}
	}
	switch e := e.(type) {
	case *ast.Ident:
		return x.evalVar(e, depth)
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

// evalVar follows a local or package variable to the values assigned to it.
func (x *extractor) evalVar(id *ast.Ident, depth int) value {
	v, ok := x.info.Uses[id].(*types.Var)
	if !ok || v.IsField() || x.visiting[v] {
		return unknown
	}
	as := x.assigns[v]
	if as == nil || as.escaped || len(as.values) == 0 {
		return unknown
	}
	x.visiting[v] = true
	defer delete(x.visiting, v)

	if a := as.values[0]; len(as.values) == 1 && !a.append && !a.tuple {
		return x.eval(a.expr, depth+1)
	}

	// The text depends on control flow, but the build method can still be
	// told: SQL appended from non-constant values is dynamic.
	result := value{method: pgguard.BuildConstant}
	for _, a := range as.values {
		if a.tuple {
			result.method = pgguard.BuildUnknown
			continue
		}
		switch v := x.eval(a.expr, depth+1); {
		case v.method == pgguard.BuildSprintf || v.method == pgguard.BuildConcat:
			return value{method: v.method}
		case v.method == pgguard.BuildUnknown && a.append:
			return value{method: pgguard.BuildConcat}
		case v.method == pgguard.BuildUnknown:
			result.method = pgguard.BuildUnknown
		}
	}
	return result
}

func (x *extractor) evalConcat(e *ast.BinaryExpr, depth int) value {
	l, r := x.eval(e.X, depth+1), x.eval(e.Y, depth+1)
	switch {
	case l.exact && r.exact:
		return value{text: l.text + r.text, method: pgguard.BuildConstant, exact: true}
	case l.method == pgguard.BuildConstant && r.method == pgguard.BuildConstant:
		return value{method: pgguard.BuildConstant}
	}
	return value{text: template(e.X, l) + template(e.Y, r), method: pgguard.BuildConcat}
}

// template renders v for display, showing non-constant parts as {expr}.
func template(e ast.Expr, v value) string {
	if v.exact || v.text != "" {
		return v.text
	}
	return "{" + types.ExprString(e) + "}"
}

// evalSprintf evaluates fmt.Sprintf calls whose arguments are all known,
// and otherwise reports the format string as a template.
func (x *extractor) evalSprintf(call *ast.CallExpr, depth int) value {
	dynamic := value{method: pgguard.BuildSprintf}
	if len(call.Args) == 0 || call.Ellipsis.IsValid() {
		return dynamic
	}
	format := x.eval(call.Args[0], depth+1)
	dynamic.text = format.text
	if !format.exact {
		return dynamic
	}
	args := make([]any, 0, len(call.Args)-1)
	for _, a := range call.Args[1:] {
		v, ok := x.constArg(a, depth+1)
		if !ok {
			return dynamic
		}
		args = append(args, v)
	}
	return value{text: fmt.Sprintf(format.text, args...), method: pgguard.BuildConstant, exact: true}
}

// constArg returns a Sprintf argument whose value is known at compile time.
func (x *extractor) constArg(e ast.Expr, depth int) (any, bool) {
	tv := x.info.Types[ast.Unparen(e)]
	if tv.Type == nil || types.NewMethodSet(tv.Type).Len() > 0 {
		return nil, false // fmt might call a String or Error method
	}
	if tv.Value != nil {
		switch tv.Value.Kind() {
		case constant.String:
			return constant.StringVal(tv.Value), true
		case constant.Bool:
			return constant.BoolVal(tv.Value), true
		case constant.Int:
			return constant.Int64Val(tv.Value)
		}
		return nil, false
	}
	if isString(tv.Type) {
		if v := x.eval(e, depth); v.exact {
			return v.text, true
		}
	}
	return nil, false
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

// assignment is a value stored in a variable.
type assignment struct {
	expr   ast.Expr
	append bool // +=
	tuple  bool // one result of a multi-value expression, or a range variable
}

type assignments struct {
	values  []assignment
	escaped bool // the variable's address is taken
}

// indexAssignments records the values assigned to each variable in files.
// Declarations without a value are skipped: an empty string is not SQL.
func indexAssignments(files []*ast.File, info *types.Info) map[*types.Var]*assignments {
	index := make(map[*types.Var]*assignments)
	get := func(e ast.Expr) *assignments {
		v := varOf(info, e)
		if v == nil {
			return nil
		}
		as := index[v]
		if as == nil {
			as = &assignments{}
			index[v] = as
		}
		return as
	}
	add := func(e ast.Expr, a assignment) {
		if as := get(e); as != nil {
			as.values = append(as.values, a)
		}
	}

	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				switch {
				case n.Tok == token.ADD_ASSIGN:
					add(n.Lhs[0], assignment{expr: n.Rhs[0], append: true})
				case n.Tok != token.ASSIGN && n.Tok != token.DEFINE:
				case len(n.Lhs) == len(n.Rhs):
					for i, lhs := range n.Lhs {
						add(lhs, assignment{expr: n.Rhs[i]})
					}
				default:
					for _, lhs := range n.Lhs {
						add(lhs, assignment{expr: n.Rhs[0], tuple: true})
					}
				}
			case *ast.ValueSpec:
				switch {
				case len(n.Values) == len(n.Names):
					for i, name := range n.Names {
						add(name, assignment{expr: n.Values[i]})
					}
				case len(n.Values) == 1:
					for _, name := range n.Names {
						add(name, assignment{expr: n.Values[0], tuple: true})
					}
				}
			case *ast.RangeStmt:
				for _, e := range []ast.Expr{n.Key, n.Value} {
					if e != nil {
						add(e, assignment{tuple: true})
					}
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND {
					if as := get(n.X); as != nil {
						as.escaped = true
					}
				}
			}
			return true
		})
	}
	return index
}

func varOf(info *types.Info, e ast.Expr) *types.Var {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok || id.Name == "_" {
		return nil
	}
	if v, ok := info.Defs[id].(*types.Var); ok {
		return v
	}
	v, _ := info.Uses[id].(*types.Var)
	return v
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
