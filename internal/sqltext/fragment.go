package sqltext

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
