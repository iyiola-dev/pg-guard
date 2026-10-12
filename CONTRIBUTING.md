# Contributing to pg-guard

Thanks for your interest in contributing!

## Getting Started

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally:
   ```sh
   git clone https://github.com/<your-username>/pg-guard.git
   cd pg-guard
   ```
3. **Create a branch** for your change:
   ```sh
   git checkout -b my-feature
   ```
4. **Install dependencies**:
   ```sh
   go mod tidy
   ```

## Making Changes

- Keep changes focused — one feature or fix per PR.
- Follow existing code style and conventions.
- Add or update tests for any new functionality.
- Run the full test suite before submitting:
  ```sh
  go test ./...
  ```
  Building needs Go 1.25+ with cgo and a C compiler. Integration tests require Docker (uses [testcontainers-go](https://golang.testcontainers.org/)); `go test -short ./...` skips them.

### Analyzer test cases

`testdata/src` is a GOPATH-style tree shared by the analyzer tests:

- `lint/...` and `dblint/` hold Go code checked by `plugin/plugin_test.go`. Each expected diagnostic is marked with a `// want` comment ([analysistest](https://pkg.go.dev/golang.org/x/tools/go/analysis/analysistest) syntax), and any diagnostic without one fails the test, so add cases for code that must *not* be reported too.
- `extract/` holds calls whose extracted details `internal/extractor` checks, each marked with an `// @name` comment.
- `github.com/jackc/pgx/v5/...` and `gorm.io/gorm/...` are minimal stand-ins for pgx and GORM with the same import paths and method signatures, so tests run without those modules.

### Go documentation and comments

Use these Go documentation conventions:

- Document public packages, preferably in a `doc.go` file.
- Start exported declaration comments with the declaration's name and use complete sentences.
- Describe behavior, guarantees, and limitations. Use internal comments to explain intent, invariants, and non-obvious constraints; omit comments that restate the code.
- Prefer testable examples to long code snippets in documentation comments.

## Submitting a Pull Request

1. **Push** your branch to your fork:
   ```sh
   git push origin my-feature
   ```
2. **Open a Pull Request** against the `main` branch of the upstream repository.
3. Describe your changes clearly in the PR description.
4. Link any related issues.

## Reporting Issues

- Use GitHub Issues to report bugs or request features.
- Include Go version, OS, and steps to reproduce when reporting bugs.

## Code of Conduct

Be respectful and constructive. We're all here to build something useful.
