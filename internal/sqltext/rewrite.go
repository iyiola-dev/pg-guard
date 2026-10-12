package sqltext

import (
	"strconv"
	"strings"
)

// Placeholders describes bind-parameter syntax other than Postgres' $N.
type Placeholders struct {
	// Question is how many '?' placeholders are bound, counting from the
	// start of the text as GORM does; -1 binds every one.
	Question int
	// Named binds @name placeholders (GORM named arguments, pgx.NamedArgs).
	Named bool
}

// Rewrite rewrites the placeholders described by p to $1, $2, ... so that
// the Postgres parser and planner accept the text. Quoted strings, quoted
// identifiers and comments are left alone. A '?' directly after IN becomes
// ($N), matching how GORM expands slice arguments.
func Rewrite(sql string, p Placeholders) string {
	if p.Question == 0 && !p.Named {
		return sql
	}

	var (
		b        strings.Builder
		params   int
		bound    int
		names    = make(map[string]int)
		lastWord string
	)
	param := func(n int) {
		b.WriteByte('$')
		b.WriteString(strconv.Itoa(n))
	}

	for i := 0; i < len(sql); {
		c := sql[i]
		var next byte
		if i+1 < len(sql) {
			next = sql[i+1]
		}

		switch {
		case c == '\'':
			escapes := i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') && (i < 2 || !isIdent(sql[i-2]))
			end := skipQuoted(sql, i, '\'', escapes)
			b.WriteString(sql[i:end])
			i, lastWord = end, ""
		case c == '"':
			end := skipQuoted(sql, i, '"', false)
			b.WriteString(sql[i:end])
			i, lastWord = end, ""
		case c == '-' && next == '-':
			end := len(sql)
			if nl := strings.IndexByte(sql[i:], '\n'); nl >= 0 {
				end = i + nl
			}
			b.WriteString(sql[i:end])
			i = end
		case c == '/' && next == '*':
			end := skipBlockComment(sql, i)
			b.WriteString(sql[i:end])
			i = end
		case c == '$' && (i == 0 || !isIdent(sql[i-1])):
			end := dollarQuoteEnd(sql, i)
			if end < 0 {
				end = i + 1
			}
			b.WriteString(sql[i:end])
			i, lastWord = end, ""
		case c == '?' && p.Question != 0 && (p.Question < 0 || bound < p.Question):
			bound++
			params++
			if strings.EqualFold(lastWord, "in") {
				b.WriteByte('(')
				param(params)
				b.WriteByte(')')
			} else {
				param(params)
			}
			i, lastWord = i+1, ""
		case c == '@' && p.Named && isNameStart(next):
			end := i + 1
			for end < len(sql) && (isNameStart(sql[end]) || isDigit(sql[end])) {
				end++
			}
			name := sql[i+1 : end]
			n, ok := names[name]
			if !ok {
				params++
				n = params
				names[name] = n
			}
			param(n)
			i, lastWord = end, ""
		case isIdentStart(c):
			end := i + 1
			for end < len(sql) && isIdent(sql[end]) {
				end++
			}
			lastWord = sql[i:end]
			b.WriteString(lastWord)
			i = end
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			b.WriteByte(c)
			i++
		default:
			b.WriteByte(c)
			i, lastWord = i+1, ""
		}
	}
	return b.String()
}

// skipQuoted returns the index just past the quoted text starting at s[i].
// A doubled quote character is an escaped quote.
func skipQuoted(s string, i int, quote byte, backslash bool) int {
	for i++; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if backslash {
				i++
			}
		case quote:
			if i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(s)
}

// skipBlockComment returns the index just past the (possibly nested) block
// comment starting at s[i].
func skipBlockComment(s string, i int) int {
	depth := 0
	for i < len(s) {
		switch {
		case strings.HasPrefix(s[i:], "/*"):
			depth++
			i += 2
		case strings.HasPrefix(s[i:], "*/"):
			depth--
			i += 2
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}
	return len(s)
}

// dollarQuoteEnd returns the index just past the dollar-quoted string
// starting at s[i], or -1 if s[i] does not start one.
func dollarQuoteEnd(s string, i int) int {
	j := i + 1
	if j < len(s) && isIdentStart(s[j]) {
		for j < len(s) && (isIdentStart(s[j]) || isDigit(s[j])) {
			j++
		}
	}
	if j >= len(s) || s[j] != '$' {
		return -1
	}
	tag := s[i : j+1]
	end := strings.Index(s[j+1:], tag)
	if end < 0 {
		return len(s)
	}
	return j + 1 + end + len(tag)
}

func isIdentStart(c byte) bool {
	return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c >= 0x80
}

func isIdent(c byte) bool {
	return isIdentStart(c) || isDigit(c) || c == '$'
}

func isNameStart(c byte) bool {
	return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}
