package unrelated

import (
	"net/http"
	"regexp"
)

// builder has methods named like database methods but is not one.
type builder struct{}

func (builder) Where(cond string) builder          { return builder{} }
func (builder) Query(q string, args ...any) string { return q }
func (builder) Exec(s string) error                { return nil }
func (builder) Raw(s string) builder               { return builder{} }

func handler(w http.ResponseWriter, r *http.Request, items []string) {
	for range items {
		_ = r.URL.Query().Get("x")
		b := builder{}
		_ = b.Where("not sql at all").Query("SELEC")
		_ = b.Raw("?").Exec("whatever")
	}
	_ = regexp.MustCompile("SELEC").FindString("y")
}
