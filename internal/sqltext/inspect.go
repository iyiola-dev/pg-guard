package sqltext

import (
	"strings"

	pgquery "github.com/pganalyze/pg_query_go/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Clauses a GORM fragment can belong to.
const (
	ClauseWhere   = "WHERE"
	ClauseHaving  = "HAVING"
	ClauseOrderBy = "ORDER BY"
	ClauseGroupBy = "GROUP BY"
	ClauseSelect  = "SELECT"
	ClauseFrom    = "FROM"
	ClauseJoin    = "JOIN"
)

// Statement embeds a clause fragment in a minimal statement so that the
// Postgres parser can check it.
func Statement(fragment, clause string) string {
	switch clause {
	case ClauseSelect:
		return "SELECT " + fragment + "\n"
	case ClauseFrom:
		return "SELECT 1 FROM " + fragment + "\n"
	case ClauseJoin:
		return "SELECT 1 FROM t " + fragment + "\n"
	default:
		return "SELECT 1 " + clause + " " + fragment + "\n"
	}
}

// Table is a relation named in a statement.
type Table struct {
	Schema string
	Name   string
}

// String returns the table as a SQL name, quoting the parts that need it.
func (t Table) String() string {
	if t.Schema == "" {
		return quoteIdent(t.Name)
	}
	return quoteIdent(t.Schema) + "." + quoteIdent(t.Name)
}

// Info describes parsed SQL.
type Info struct {
	Statements int
	// Command is SELECT, INSERT, UPDATE, DELETE or MERGE when the text is a
	// single such statement, and empty otherwise.
	Command string
	// Tables are the relations Command reads or writes, excluding CTEs.
	Tables []Table
	// Created are the relations created by CREATE TABLE, CREATE VIEW and
	// SELECT INTO statements.
	Created []Table
	// Params is the highest $N placeholder number.
	Params int
}

// Inspect parses sql with the Postgres parser. A returned error describes
// a syntax error.
func Inspect(sql string) (*Info, error) {
	tree, err := pgquery.Parse(sql)
	if err != nil {
		return nil, err
	}

	info := &Info{Statements: len(tree.Stmts)}
	for _, raw := range tree.Stmts {
		walk(raw.GetStmt(), func(m proto.Message) bool {
			if p, ok := m.(*pgquery.ParamRef); ok {
				info.Params = max(info.Params, int(p.Number))
			}
			return true
		})
		if t, ok := created(raw.GetStmt()); ok {
			info.Created = append(info.Created, t)
		}
	}
	if len(tree.Stmts) == 1 {
		stmt := tree.Stmts[0].GetStmt()
		if info.Command = command(stmt); info.Command != "" {
			info.Tables = tables(stmt)
		}
	}
	return info, nil
}

// FilterColumns returns the columns referenced by a filter condition from
// an EXPLAIN plan, in order of first appearance.
func FilterColumns(filter string) []string {
	tree, err := pgquery.Parse("SELECT 1 WHERE " + filter)
	if err != nil {
		return nil
	}

	var cols []string
	seen := make(map[string]bool)
	for _, raw := range tree.Stmts {
		walk(raw.GetStmt(), func(m proto.Message) bool {
			ref, ok := m.(*pgquery.ColumnRef)
			if !ok || len(ref.Fields) == 0 {
				return true
			}
			// Earlier fields qualify the column with a table or alias.
			name := ref.Fields[len(ref.Fields)-1].GetString_().GetSval()
			if name != "" && !seen[name] {
				seen[name] = true
				cols = append(cols, name)
			}
			return true
		})
	}
	return cols
}

func command(stmt *pgquery.Node) string {
	switch n := stmt.GetNode().(type) {
	case *pgquery.Node_SelectStmt:
		if n.SelectStmt.GetIntoClause() == nil {
			return "SELECT"
		}
	case *pgquery.Node_InsertStmt:
		return "INSERT"
	case *pgquery.Node_UpdateStmt:
		return "UPDATE"
	case *pgquery.Node_DeleteStmt:
		return "DELETE"
	case *pgquery.Node_MergeStmt:
		return "MERGE"
	}
	return ""
}

func created(stmt *pgquery.Node) (Table, bool) {
	var rel *pgquery.RangeVar
	switch n := stmt.GetNode().(type) {
	case *pgquery.Node_CreateStmt:
		rel = n.CreateStmt.GetRelation()
	case *pgquery.Node_CreateTableAsStmt:
		rel = n.CreateTableAsStmt.GetInto().GetRel()
	case *pgquery.Node_ViewStmt:
		rel = n.ViewStmt.GetView()
	case *pgquery.Node_SelectStmt:
		rel = n.SelectStmt.GetIntoClause().GetRel()
	}
	if rel == nil {
		return Table{}, false
	}
	return tableOf(rel), true
}

func tables(stmt *pgquery.Node) []Table {
	ctes := make(map[string]bool)
	var refs []Table
	walk(stmt, func(m proto.Message) bool {
		switch n := m.(type) {
		case *pgquery.CommonTableExpr:
			ctes[n.Ctename] = true
		case *pgquery.RangeVar:
			refs = append(refs, tableOf(n))
		case *pgquery.LockingClause:
			return false // FOR UPDATE OF lists aliases, not tables
		}
		return true
	})

	var out []Table
	seen := make(map[Table]bool)
	for _, t := range refs {
		if (t.Schema == "" && ctes[t.Name]) || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func tableOf(rel *pgquery.RangeVar) Table {
	return Table{Schema: rel.GetSchemaname(), Name: rel.GetRelname()}
}

// walk calls visit for m and every message below it, skipping the
// children of messages for which visit returns false.
func walk(m proto.Message, visit func(proto.Message) bool) {
	rm := m.ProtoReflect()
	if !rm.IsValid() || !visit(m) {
		return
	}
	rm.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList() && fd.Message() != nil:
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				walk(list.Get(i).Message().Interface(), visit)
			}
		case fd.Message() != nil && !fd.IsMap():
			walk(v.Message().Interface(), visit)
		}
		return true
	})
}

// quoteIdent quotes name unless Postgres would read it unchanged without
// quotes.
func quoteIdent(name string) string {
	plain := name != ""
	for i := 0; i < len(name) && plain; i++ {
		c := name[i]
		plain = c == '_' || 'a' <= c && c <= 'z' || i > 0 && (isDigit(c) || c == '$')
	}
	if plain {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
