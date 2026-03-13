package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/analyzer"
	"github.com/iyiola-dev/pg-guard/internal/report"
)

func main() {
	dsn := flag.String("dsn", "", "Postgres connection string for live analysis")
	format := flag.String("format", "text", "Output format: text or json")
	flag.Parse()

	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	cfg := pgguard.Config{
		Patterns: patterns,
		DSN:      *dsn,
		Format:   *format,
	}

	ctx := context.Background()
	findings, err := analyzer.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pg-guard: %v\n", err)
		os.Exit(2)
	}

	switch cfg.Format {
	case "json":
		if err := report.JSON(findings, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "pg-guard: %v\n", err)
			os.Exit(2)
		}
	default:
		if err := report.Text(findings, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "pg-guard: %v\n", err)
			os.Exit(2)
		}
	}

	if hasErrors(findings) {
		os.Exit(1)
	}
}

func hasErrors(findings []pgguard.Finding) bool {
	for _, f := range findings {
		if f.Severity == pgguard.SeverityError {
			return true
		}
	}
	return false
}
