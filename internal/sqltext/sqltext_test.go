package sqltext

import "testing"

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
