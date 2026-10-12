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

// Settings configures the analyzer. golangci-lint reads these settings from
// linters.settings.custom.pgguard.settings.
type Settings struct {
	// DSN is the Postgres connection string for live database checks.
	// An empty value uses PG_GUARD_DSN; if both are empty, only static checks run.
	DSN string `json:"dsn"`
}

// Analyzer runs pg-guard's static checks and enables live database checks when
// PG_GUARD_DSN is set on its first run.
var Analyzer = NewAnalyzer(Settings{})

// NewAnalyzer returns an analyzer configured with s.
// It connects to Postgres on its first run when live database checks are enabled.
func NewAnalyzer(s Settings) *analysis.Analyzer {
	r := &runner{settings: s}
	return &analysis.Analyzer{
		Name: "pgguard",
		Doc:  "Postgres query linter and risk analyzer",
		Run:  r.run,
	}
}

// New returns a golangci-lint module plugin configured from conf.
// It returns an error if conf contains invalid or unknown settings.
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

// BuildAnalyzers returns the analyzers configured with the plugin's settings.
func (l *linter) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{NewAnalyzer(l.settings)}, nil
}

// GetLoadMode returns the load mode needed to identify database calls by type.
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

// dbChecker initializes the live database checker once for this runner.
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
