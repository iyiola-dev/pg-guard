package plugin

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/golangci/plugin-module-register/register"
	"github.com/iyiola-dev/pg-guard/internal/analyzer"
	"github.com/iyiola-dev/pg-guard/internal/dbinfo"
	"github.com/iyiola-dev/pg-guard/internal/extractor"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("pgguard", New)
}

// Settings configure the analyzer. golangci-lint reads them from
// linters.settings.custom.pgguard.settings.
type Settings struct {
	// DSN enables the live database checks. When empty, PG_GUARD_DSN is used.
	DSN string `json:"dsn"`
}

// Analyzer runs pg-guard's static checks, and its live database checks
// when PG_GUARD_DSN is set.
var Analyzer = NewAnalyzer(Settings{})

func NewAnalyzer(s Settings) *analysis.Analyzer {
	r := &runner{settings: s}
	return &analysis.Analyzer{
		Name: "pgguard",
		Doc:  "Postgres query linter and risk analyzer",
		Run:  r.run,
	}
}

// New is the entry point for golangci-lint's module plugin system.
func New(conf any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[Settings](conf)
	if err != nil {
		return nil, err
	}
	return &linter{settings: s}, nil
}

type linter struct {
	settings Settings
}

func (l *linter) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{NewAnalyzer(l.settings)}, nil
}

func (l *linter) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

type runner struct {
	settings Settings

	once    sync.Once
	checker *analyzer.DBChecker
	err     error
}

func (r *runner) run(pass *analysis.Pass) (interface{}, error) {
	queries := extractor.Extract(pass.Fset, pass.Files, pass.TypesInfo)
	findings := analyzer.StaticChecks(queries)

	checker, err := r.dbChecker()
	if err != nil {
		return nil, err
	}
	if checker != nil {
		dbFindings, err := checker.Check(context.Background(), queries)
		if err != nil {
			return nil, err
		}
		findings = append(findings, dbFindings...)
	}

	for _, f := range findings {
		pass.Report(analysis.Diagnostic{
			Pos:      f.TokenPos,
			Category: string(f.Check),
			Message:  fmt.Sprintf("[%s] %s", f.Check, f.Message),
		})
	}
	return nil, nil
}

// dbChecker connects on first use and shares the connection between
// packages for the life of the process.
func (r *runner) dbChecker() (*analyzer.DBChecker, error) {
	r.once.Do(func() {
		dsn := r.settings.DSN
		if dsn == "" {
			dsn = os.Getenv("PG_GUARD_DSN")
		}
		if dsn == "" {
			return
		}
		db, err := dbinfo.New(context.Background(), dsn)
		if err != nil {
			r.err = err
			return
		}
		r.checker = analyzer.NewDBChecker(db)
	})
	return r.checker, r.err
}
