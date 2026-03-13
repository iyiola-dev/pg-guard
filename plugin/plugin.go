package plugin

import (
	"context"
	"go/ast"
	"os"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/analyzer"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "pgguard",
	Doc:  "Postgres query linter and risk analyzer",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	dsn := os.Getenv("PG_GUARD_DSN")
	cfg := pgguard.Config{
		Patterns: []string{pass.Pkg.Path()},
		DSN:      dsn,
	}

	findings, err := analyzer.Run(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	for _, f := range findings {
		for _, file := range pass.Files {
			if pass.Fset.Position(file.Pos()).Filename == f.Pos.Filename {
				reportFinding(pass, file, f)
				break
			}
		}
	}

	return nil, nil
}

func reportFinding(pass *analysis.Pass, _ *ast.File, f pgguard.Finding) {
	pos := pass.Fset.File(pass.Files[0].Pos()).Pos(f.Pos.Offset)

	switch f.Severity {
	case pgguard.SeverityError:
		pass.Reportf(pos, "[%s] %s", f.Check, f.Message)
	default:
		pass.Reportf(pos, "[%s] %s", f.Check, f.Message)
	}
}

// AnalyzerPlugin is the entry point for golangci-lint.
func AnalyzerPlugin() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{Analyzer}, nil
}
