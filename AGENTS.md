# Repository Guidelines

## Project Structure & Module Organization

The root `catalogue` package lives in `catalogue.go`; its tests live in `catalogue_test.go`. `registry.v1.json` is embedded and holds canonical symbols, aliases, version metadata, and provenance. Keep its grammar and maintenance notes synchronized with `REGISTRY.md`.

`cmd/catalogue-eval/` contains the command-line evaluator and its tests. It reads an external MusicBrainz JSONL snapshot and writes ignored local review state to `.catalogue-eval/`. General usage belongs in `README.md`.

## Build, Test, and Development Commands

- `go test ./...` runs all package and CLI tests.
- `go vet ./...` performs standard Go static checks.
- `gofmt -w catalogue.go catalogue_test.go cmd/catalogue-eval/*.go` formats edited Go files.
- `go build ./...` verifies every package and command compiles.
- `go run ./cmd/catalogue-eval sample -input /path/to/catalogue-references.jsonl` starts a local precision evaluation; use the `review` and `summary` subcommands afterward.

## Coding Style & Naming Conventions

Follow idiomatic Go and `gofmt` output (tabs for indentation). Use short, descriptive lower-case names for unexported identifiers and PascalCase for exported APIs. Every exported type, function, or method should have a Go doc comment beginning with its name. Keep parser behavior deterministic, avoid runtime network or filesystem dependencies in the library, and preserve caller ownership of returned mutable data.

## Testing Guidelines

Use Go's `testing` package. Name tests `TestBehavior`, prefer table-driven subtests for grammar cases, and call `t.Parallel()` where cases share no mutable state. Add positive, rejection, normalization, Unicode, and byte-span cases when parser boundaries change. Registry edits must retain sorted entries and update metadata, expected counts/hashes, and documentation. No coverage threshold is enforced; new behavior must have focused regression tests.

## Agent Workflow

Verify only claims the user lists. Inspect the minimum repository surface needed to justify a fix, and make the smallest safe change. Expand exploration only when the task explicitly requires it. If a prompt is too vague to act on, use a question tool or stop and ask for clarification.

## Commit & Pull Request Guidelines

Use Conventional Commits with an imperative description, for example `feat(parser): expose match byte spans`, `fix(registry): reject duplicate aliases`, or `docs: clarify evaluation workflow`. Use `!` and a `BREAKING CHANGE:` footer for incompatible changes. Multiple scoped commits are allowed when separation aids clarity and future reviewability; explain non-obvious motivation in bodies. Pull requests should describe observable behavior and compatibility impact, list verification commands, link issues, and call out registry provenance or schema/revision changes. Never commit external datasets or `.catalogue-eval/` review artifacts.
