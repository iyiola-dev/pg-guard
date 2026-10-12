package sqltext

import (
	"slices"
	"testing"
)

func TestRewrite(t *testing.T) {
	gorm := func(n int) Placeholders { return Placeholders{Question: n} }
	tests := []struct {
		name string
		sql  string
		p    Placeholders
		want string
	}{
		{"dollar untouched", "SELECT $1, '?'", Placeholders{}, "SELECT $1, '?'"},
		{"question marks", "a = ? AND b = ?", gorm(-1), "a = $1 AND b = $2"},
		{"bound count", "a = ? AND b ?| c", gorm(1), "a = $1 AND b ?| c"},
		{"IN expands", "id IN ? OR id in (?)", gorm(-1), "id IN ($1) OR id in ($2)"},
		{"NOT IN with comment", "id NOT IN /* ids */ ?", gorm(-1), "id NOT IN /* ids */ ($1)"},
		{"single quotes", "a = '?' AND b = 'it''s ?' AND c = ?", gorm(-1), "a = '?' AND b = 'it''s ?' AND c = $1"},
		{"escape string", `a = E'\'?' AND b = ?`, gorm(-1), `a = E'\'?' AND b = $1`},
		{"double quotes", `"we?ird" = ?`, gorm(-1), `"we?ird" = $1`},
		{"line comment", "a = ? -- why?\nAND b = ?", gorm(-1), "a = $1 -- why?\nAND b = $2"},
		{"nested block comment", "/* a /* ? */ ? */ b = ?", gorm(-1), "/* a /* ? */ ? */ b = $1"},
		{"dollar quotes", "a = $$?$$ AND b = $tag$ ? $tag$ AND c = ?", gorm(-1), "a = $$?$$ AND b = $tag$ ? $tag$ AND c = $1"},
		{"named", "a = @name OR b = @other OR c = @name", Placeholders{Named: true}, "a = $1 OR b = $2 OR c = $1"},
		{"named skips operators", "tags @> @tags AND @@ x AND e = 'a@b.c'", Placeholders{Named: true}, "tags @> $1 AND @@ x AND e = 'a@b.c'"},
		{"mixed", "a = ? AND b = @b", Placeholders{Question: -1, Named: true}, "a = $1 AND b = $2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Rewrite(tt.sql, tt.p); got != tt.want {
				t.Errorf("Rewrite(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

func TestStatement(t *testing.T) {
	fragments := []struct{ fragment, clause string }{
		{"name = $1 AND age > $2", ClauseWhere},
		{"count(*) > $1", ClauseHaving},
		{"name desc, id", ClauseOrderBy},
		{"name", ClauseGroupBy},
		{"name, count(*) AS total", ClauseSelect},
		{"users AS u", ClauseFrom},
		{"LEFT JOIN emails ON emails.user_id = users.id", ClauseJoin},
		{"name = $1 -- trailing comment", ClauseWhere},
	}
	for _, f := range fragments {
		if _, err := Inspect(Statement(f.fragment, f.clause)); err != nil {
			t.Errorf("%s fragment %q: %v", f.clause, f.fragment, err)
		}
	}
	if _, err := Inspect(Statement("name = $1 AND", ClauseWhere)); err == nil {
		t.Error("expected a syntax error for an incomplete WHERE fragment")
	}
}

func TestInspect(t *testing.T) {
	tests := []struct {
		sql        string
		statements int
		command    string
		tables     []Table
		created    []Table
		params     int
	}{
		{
			sql: "SELECT u.id FROM users u JOIN analytics.events e ON e.user_id = u.id WHERE u.id = $2", statements: 1, command: "SELECT",
			tables: []Table{{Name: "users"}, {Schema: "analytics", Name: "events"}}, params: 2,
		},
		{
			sql:        `WITH recent AS (SELECT * FROM "Orders" WHERE created_at > $1) SELECT * FROM recent FOR UPDATE OF recent`,
			statements: 1, command: "SELECT", tables: []Table{{Name: "Orders"}}, params: 1,
		},
		{sql: "INSERT INTO users (name) SELECT name FROM staging", statements: 1, command: "INSERT", tables: []Table{{Name: "users"}, {Name: "staging"}}},
		{sql: "UPDATE users SET name = $1 FROM teams WHERE teams.id = users.team_id", statements: 1, command: "UPDATE", tables: []Table{{Name: "users"}, {Name: "teams"}}, params: 1},
		{sql: "DELETE FROM users WHERE id = $1", statements: 1, command: "DELETE", tables: []Table{{Name: "users"}}, params: 1},
		{sql: "SELECT * FROM generate_series(1, 3)", statements: 1, command: "SELECT"},
		{sql: "CREATE TEMP TABLE scratch (id int)", statements: 1, created: []Table{{Name: "scratch"}}},
		{sql: "SELECT * INTO archive FROM users", statements: 1, created: []Table{{Name: "archive"}}},
		{sql: "CREATE VIEW v AS SELECT 1; CREATE TABLE t AS SELECT 1", statements: 2, created: []Table{{Name: "v"}, {Name: "t"}}},
		{sql: "SELECT 1; DROP TABLE users", statements: 2},
		{sql: "-- nothing here", statements: 0},
	}
	for _, tt := range tests {
		info, err := Inspect(tt.sql)
		if err != nil {
			t.Errorf("Inspect(%q): %v", tt.sql, err)
			continue
		}
		if info.Statements != tt.statements || info.Command != tt.command || info.Params != tt.params ||
			!slices.Equal(info.Tables, tt.tables) || !slices.Equal(info.Created, tt.created) {
			t.Errorf("Inspect(%q) = %+v", tt.sql, *info)
		}
	}

	if _, err := Inspect("SELEC 1"); err == nil {
		t.Error("expected a syntax error")
	}
}

func TestFilterColumns(t *testing.T) {
	tests := []struct {
		filter string
		want   []string
	}{
		{"(users.email = $1)", []string{"email"}},
		{"((lower((u.name)::text) ~~ 'a%'::text) AND (u.deleted_at IS NULL) AND (u.name <> ''::text))", []string{"name", "deleted_at"}},
		{"(NOT (hashed SubPlan 1))", nil},
	}
	for _, tt := range tests {
		if got := FilterColumns(tt.filter); !slices.Equal(got, tt.want) {
			t.Errorf("FilterColumns(%q) = %q, want %q", tt.filter, got, tt.want)
		}
	}
}

func TestTableString(t *testing.T) {
	tests := []struct {
		table Table
		want  string
	}{
		{Table{Name: "users"}, "users"},
		{Table{Schema: "public", Name: "users"}, "public.users"},
		{Table{Name: "Users"}, `"Users"`},
		{Table{Schema: "my schema", Name: `we"ird`}, `"my schema"."we""ird"`},
		{Table{Name: "t1$"}, "t1$"},
	}
	for _, tt := range tests {
		if got := tt.table.String(); got != tt.want {
			t.Errorf("%#v.String() = %s, want %s", tt.table, got, tt.want)
		}
	}
}
