package extractor

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	pgguard "github.com/iyiola-dev/pg-guard"
	"golang.org/x/tools/go/packages"
)

// loadMarked extracts the queries of testdata/src/extract and indexes them
// by the @name marker on their line.
func loadMarked(t *testing.T) map[string][]pgguard.ExtractedQuery {
	t.Helper()
	gopath, err := filepath.Abs(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  gopath,
		Env:  append(os.Environ(), "GOPATH="+gopath, "GO111MODULE=off", "GOWORK=off", "GOFLAGS="),
	}
	pkgs, err := packages.Load(cfg, "extract")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("load: %v", pkgs)
	}
	pkg := pkgs[0]

	markers := make(map[int]string)
	marker := regexp.MustCompile(`// @(\w+)`)
	f, err := os.Open(pkg.GoFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		if m := marker.FindStringSubmatch(scanner.Text()); m != nil {
			markers[line] = m[1]
		}
	}

	byMarker := make(map[string][]pgguard.ExtractedQuery)
	for _, q := range Extract(pkg.Fset, pkg.Syntax, pkg.TypesInfo) {
		if name, ok := markers[q.Pos.Line]; ok {
			byMarker[name] = append(byMarker[name], q)
		}
	}
	return byMarker
}

func TestExtract(t *testing.T) {
	queries := loadMarked(t)
	get := func(marker, funcName string) pgguard.ExtractedQuery {
		t.Helper()
		for _, q := range queries[marker] {
			if q.FuncName == funcName {
				return q
			}
		}
		t.Fatalf("@%s: no %s call extracted (got %+v)", marker, funcName, queries[marker])
		return pgguard.ExtractedQuery{}
	}

	tests := []struct {
		marker, funcName string
		want             pgguard.ExtractedQuery
	}{
		{"escaped", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT \"name\"\nFROM users", NormalizedSQL: "SELECT \"name\"\nFROM users", BuildMethod: pgguard.BuildConstant}},
		{"backtick", "QueryContext", pgguard.ExtractedQuery{
			SQL: `SELECT "id" FROM "users"`, NormalizedSQL: `SELECT "id" FROM "users"`, BuildMethod: pgguard.BuildConstant}},
		{"constconcat", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT id, name FROM users", NormalizedSQL: "SELECT id, name FROM users", BuildMethod: pgguard.BuildConstant}},
		{"dynconcat", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT id FROM users WHERE name = '{name}'", BuildMethod: pgguard.BuildConcat}},
		{"constsprintf", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT id, name FROM users LIMIT 10", NormalizedSQL: "SELECT id, name FROM users LIMIT 10", BuildMethod: pgguard.BuildConstant}},
		{"dynsprintf", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT id FROM users LIMIT %d", BuildMethod: pgguard.BuildSprintf}},
		{"assigned", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT id FROM users WHERE id = $1", NormalizedSQL: "SELECT id FROM users WHERE id = $1", BuildMethod: pgguard.BuildConstant}},
		{"pgxnamed", "Query", pgguard.ExtractedQuery{
			API: pgguard.APIPgx, SQL: "SELECT id FROM users WHERE name = @name OR alias = @name",
			NormalizedSQL: "SELECT id FROM users WHERE name = $1 OR alias = $1", BuildMethod: pgguard.BuildConstant}},
		{"gormwhere", "Where", pgguard.ExtractedQuery{
			API: pgguard.APIGORM, Call: pgguard.CallBuild, Kind: pgguard.SQLFragment, Clause: "WHERE",
			SQL:           "name = ? AND id IN ? AND note = '?'",
			NormalizedSQL: "name = $1 AND id IN ($2) AND note = '?'", BuildMethod: pgguard.BuildConstant}},
		{"gormwhere", "Find", pgguard.ExtractedQuery{API: pgguard.APIGORM}},
		{"gormnamed", "Raw", pgguard.ExtractedQuery{
			API: pgguard.APIGORM, Call: pgguard.CallBuild, SQL: "SELECT * FROM users WHERE name = @name",
			NormalizedSQL: "SELECT * FROM users WHERE name = $1", BuildMethod: pgguard.BuildConstant}},
		{"prepare", "Prepare", pgguard.ExtractedQuery{
			Call: pgguard.CallPrepare, SQL: "SELECT 1", NormalizedSQL: "SELECT 1", BuildMethod: pgguard.BuildConstant}},
		{"stmt", "Query", pgguard.ExtractedQuery{}},
		{"rangebody", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT $1::int", NormalizedSQL: "SELECT $1::int", BuildMethod: pgguard.BuildConstant}},
		{"forinit", "QueryContext", pgguard.ExtractedQuery{
			SQL: "SELECT 1", NormalizedSQL: "SELECT 1", BuildMethod: pgguard.BuildConstant}},
	}
	for _, tt := range tests {
		q := get(tt.marker, tt.funcName)
		want := tt.want
		if want.API == "" {
			want.API = pgguard.APIDatabaseSQL
		}
		if want.Call == "" {
			want.Call = pgguard.CallQuery
		}
		if want.Kind == "" && want.SQL != "" {
			want.Kind = pgguard.SQLStatement
		}
		if q.API != want.API || q.Call != want.Call || q.Kind != want.Kind || q.Clause != want.Clause ||
			q.SQL != want.SQL || q.NormalizedSQL != want.NormalizedSQL || q.BuildMethod != want.BuildMethod {
			t.Errorf("@%s %s:\n got %+v\nwant %+v", tt.marker, tt.funcName, q, want)
		}
	}

	contexts := map[string]bool{"escaped": true, "pgxnamed": true, "prepare": false, "stmt": false}
	for marker, want := range contexts {
		if q := queries[marker][0]; q.HasContext != want {
			t.Errorf("@%s: HasContext = %v, want %v", marker, q.HasContext, want)
		}
	}
	// A GORM chain on a *gorm.DB parameter may carry a context set by the caller.
	if q := get("gormwhere", "Find"); !q.HasContext {
		t.Error("@gormwhere Find: HasContext = false for a chain of unknown origin")
	}

	if q := get("rangebody", "QueryContext"); !q.InLoop {
		t.Error("@rangebody: query in a range body should be InLoop")
	}
	if q := get("forinit", "QueryContext"); q.InLoop {
		t.Error("@forinit: query in a for statement's init runs once and should not be InLoop")
	}

	// Positions point at the method name, so calls in a chain differ.
	where, find := get("gormwhere", "Where"), get("gormwhere", "Find")
	if where.Pos.Line != find.Pos.Line || where.Pos.Column >= find.Pos.Column {
		t.Errorf("chain positions: Where at %v, Find at %v", where.Pos, find.Pos)
	}
}
