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
  Integration tests require Docker (uses [testcontainers-go](https://golang.testcontainers.org/)).

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
