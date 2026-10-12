package analyzer

import (
	"cmp"
	"context"
	"fmt"
	"go/token"
	"slices"

	pgguard "github.com/iyiola-dev/pg-guard"
	"github.com/iyiola-dev/pg-guard/internal/checks"
	"github.com/iyiola-dev/pg-guard/internal/dbinfo"
	"github.com/iyiola-dev/pg-guard/internal/extractor"
	"golang.org/x/tools/go/packages"
)

func Run(ctx context.Context, cfg pgguard.Config) ([]pgguard.Finding, error) {
	pkgs, err := loadPackages(cfg.Patterns, false)
	if err != nil {
		return nil, fmt.Errorf("analyzer: load packages: %w", err)
	}

	// With tests enabled a package's files also appear in its test variant,
	// so keep one query per call site.
	var queries []pgguard.ExtractedQuery
	seen := make(map[token.Position]bool)
	for _, pkg := range pkgs {
		for _, q := range extractor.Extract(pkg.Fset, pkg.Syntax, pkg.TypesInfo) {
			if !seen[q.Pos] {
				seen[q.Pos] = true
				queries = append(queries, q)
			}
		}
	}

	findings := StaticChecks(queries)

	if cfg.DSN != "" {
		db, err := dbinfo.New(ctx, cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("analyzer: db checks: %w", err)
		}
		defer db.Close(ctx)

		dbFindings, err := NewDBChecker(db).Check(ctx, queries)
		if err != nil {
			return nil, fmt.Errorf("analyzer: db checks: %w", err)
		}
		findings = append(findings, dbFindings...)
	}

	slices.SortStableFunc(findings, func(a, b pgguard.Finding) int {
		return cmp.Or(
			cmp.Compare(a.Pos.Filename, b.Pos.Filename),
			cmp.Compare(a.Pos.Line, b.Pos.Line),
			cmp.Compare(a.Pos.Column, b.Pos.Column),
			cmp.Compare(a.Check, b.Check),
		)
	})

	return findings, nil
}

// StaticChecks runs the checks that need no database.
func StaticChecks(queries []pgguard.ExtractedQuery) []pgguard.Finding {
	var findings []pgguard.Finding
	findings = append(findings, checks.CheckUnparameterized(queries)...)
	findings = append(findings, checks.CheckMissingContext(queries)...)
	findings = append(findings, checks.CheckNPlusOne(queries)...)
	findings = append(findings, checks.CheckInvalidSQL(queries)...)
	return findings
}

func loadPackages(patterns []string, tests bool) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedSyntax | packages.NeedFiles | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo,
		Tests: tests,
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
