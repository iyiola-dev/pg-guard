package analyzer

import (
	"context"
	"fmt"
	"sort"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/checks"
	"github.com/iyiola-dev/pg-guard/internal/dbinfo"
	"github.com/iyiola-dev/pg-guard/internal/extractor"
	"golang.org/x/tools/go/packages"
)

const largeTableThreshold = 100_000

func Run(ctx context.Context, cfg pgguard.Config) ([]pgguard.Finding, error) {
	pkgs, err := loadPackages(cfg.Patterns)
	if err != nil {
		return nil, fmt.Errorf("analyzer: load packages: %w", err)
	}

	queries := extractor.Extract(pkgs)

	var findings []pgguard.Finding
	findings = append(findings, checks.CheckUnparameterized(queries)...)
	findings = append(findings, checks.CheckMissingContext(queries)...)
	findings = append(findings, checks.CheckNPlusOne(queries)...)

	if cfg.DSN != "" {
		dbFindings, err := runDBChecks(ctx, cfg.DSN, queries)
		if err != nil {
			return nil, fmt.Errorf("analyzer: db checks: %w", err)
		}
		findings = append(findings, dbFindings...)
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Pos.Filename != findings[j].Pos.Filename {
			return findings[i].Pos.Filename < findings[j].Pos.Filename
		}
		return findings[i].Pos.Line < findings[j].Pos.Line
	})

	return findings, nil
}

func loadPackages(patterns []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, fmt.Errorf("%s: %s", pkg.PkgPath, e.Msg))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("package errors: %v", errs)
	}
	return pkgs, nil
}

func runDBChecks(ctx context.Context, dsn string, queries []pgguard.ExtractedQuery) ([]pgguard.Finding, error) {
	db, err := dbinfo.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)

	var findings []pgguard.Finding

	seen := make(map[string]bool)
	for _, q := range queries {
		if q.SQL == "" {
			continue
		}
		for _, table := range q.Tables {
			if seen[table] {
				continue
			}
			seen[table] = true

			estimate, err := db.TableRowEstimate(ctx, table)
			if err != nil {
				continue
			}

			if estimate < largeTableThreshold {
				continue
			}

			result, err := db.Explain(ctx, q.SQL)
			if err != nil {
				continue
			}

			if result.SeqScan {
				findings = append(findings, pgguard.Finding{
					Pos:      q.Pos,
					Severity: pgguard.SeverityInfo,
					Check:    pgguard.CheckMissingIndex,
					Message:  fmt.Sprintf("full scan on %q (~%.1fM rows); consider adding an index", table, float64(estimate)/1e6),
					SQL:      q.SQL,
				})
			}
		}
	}

	return findings, nil
}
