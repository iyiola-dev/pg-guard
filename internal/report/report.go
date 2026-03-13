package report

import (
	"encoding/json"
	"fmt"
	"io"

	pgguard "github.com/iyiola-dev/pg-guard"
)

func Text(findings []pgguard.Finding, w io.Writer) error {
	for _, f := range findings {
		_, err := fmt.Fprintf(w, "%s:%d:%d  %-7s  %-25s  %s\n",
			f.Pos.Filename, f.Pos.Line, f.Pos.Column,
			f.Severity, f.Check, f.Message,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

type jsonFinding struct {
	File         string `json:"file"`
	Line         int    `json:"line"`
	Column       int    `json:"column"`
	Severity     string `json:"severity"`
	Check        string `json:"check"`
	Message      string `json:"message"`
	SQL          string `json:"sql,omitempty"`
	SuggestedFix string `json:"suggested_fix,omitempty"`
}

func JSON(findings []pgguard.Finding, w io.Writer) error {
	out := make([]jsonFinding, len(findings))
	for i, f := range findings {
		out[i] = jsonFinding{
			File:         f.Pos.Filename,
			Line:         f.Pos.Line,
			Column:       f.Pos.Column,
			Severity:     string(f.Severity),
			Check:        string(f.Check),
			Message:      f.Message,
			SQL:          f.SQL,
			SuggestedFix: f.SuggestedFix,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
