---
title: "Development"
description: "Build, lint and test essessey in its Docker dev image: Make targets, the 90 percent coverage gate, the security scan, CI, releases and the docs build."
nav_order: 9
---

# Development

Hacking on essessey itself. Every target runs inside the [`Dockerfile.dev`](https://github.com/psyb0t/essessey/blob/main/Dockerfile.dev) image, so a clean host needs Docker and Make rather than a curated Go toolchain. The image pins Go 1.26.10, govulncheck and semgrep; the checkout is mounted at run time, so the image is reused without a rebuild per change.

## Make targets

```bash
make dep           # tidy the module and re-vendor
make lint          # go fix, then golangci-lint at full strictness
make lint-fix      # the same, applying whatever it can fix itself
make test          # the suite, always with -race
make test-coverage # the suite plus the coverage floor CI enforces
make sec           # govulncheck + semgrep, merged into sec.sarif
make docs          # build this documentation site locally
make clean         # remove coverage and SARIF artifacts
make help          # every target with its description
```

- `make lint` fails if `go fix -diff ./...` prints anything, then runs golangci-lint from the module's own `tool` dependency in [`go.mod`](https://github.com/psyb0t/essessey/blob/main/go.mod). The linter set and limits (80-column lines, `wsl_v5`, `depguard` and the rest) are in [`.golangci.yml`](https://github.com/psyb0t/essessey/blob/main/.golangci.yml).
- `make test-coverage` runs [`scripts/test-coverage.sh`](https://github.com/psyb0t/essessey/blob/main/scripts/test-coverage.sh): race-enabled tests with coverage counted across every package, the total written to `coverage-percent.txt` for the badge, and a failure below `MIN_TEST_COVERAGE` (90).
- `make sec` runs [`scripts/sec.sh`](https://github.com/psyb0t/essessey/blob/main/scripts/sec.sh): govulncheck for reachable vulnerabilities and semgrep with the `p/golang` and `p/security-audit` rule sets (vendored code excluded), merged into `sec.sarif`. Any finding from either tool fails it.
- `make dep` runs `go mod tidy` and `go mod vendor`. Dependencies are vendored and the [`vendor/`](https://github.com/psyb0t/essessey/tree/main/vendor) tree is committed; never edit it by hand.

## Tests

Tests sit next to the code and use testify. None of them need a network, a NATS server or a WebSocket: the bindings are tested through fakes that satisfy their one-method interfaces, and through `InMemorySink` and `SliceSource` round trips. Two suites are worth knowing before you touch their code:

- [`sse/conformance_test.go`](https://github.com/psyb0t/essessey/blob/main/sse/conformance_test.go) pins the SSE format rules: multi-line payloads, the optional space, all three line endings, the BOM, the empty-data rule, last event id and retry handling.
- [`elelemstream/adapter_test.go`](https://github.com/psyb0t/essessey/blob/main/elelemstream/adapter_test.go) asserts the block indexes for text-only, thinking-then-text, single-call, parallel-call, non-zero `toolBase` and second-round cases.

## CI

[`pipeline.yml`](https://github.com/psyb0t/essessey/blob/main/.github/workflows/pipeline.yml) runs on every push to any branch, weekly on a schedule, and on demand:

| Job | When | What |
| --- | --- | --- |
| `code` | every run except tags | `make lint`, `make test-coverage` and `make sec` through the reusable `code-workflow.yml`; uploads the coverage file as an artifact and `sec.sarif` to the Security tab |
| `wait` | `v*` tags | waits for the `main` run of the tagged commit to succeed |
| `release` | `v*` tags, after `wait` | cuts the GitHub release |
| `badges` | `main` after `code` passes, or a tag after `wait` passes | refreshes the coverage, version, license and imported-by badges on the `badges` branch; a tag reuses the coverage from the `main` run |

The weekly run exists for the imported-by badge, which reads pkg.go.dev. It has to be the whole pipeline because the badge job republishes every badge from what the run produced.

## Releases

Push the commit to `main` first and let CI pass, then push an annotated `v*` tag on it. The tag run does not rerun the checks: it waits for that `main` run and releases what it tested. Add a section to [`CHANGELOG.md`](https://github.com/psyb0t/essessey/blob/main/CHANGELOG.md) that says whether the API changed; before 1.0, a minor bump may break the API and says so.

## Docs

This site is the [`docs/`](https://github.com/psyb0t/essessey/tree/main/docs) directory, published by GitHub Pages from `main`. `make docs` builds it with the same image GitHub uses, into the gitignored directory named by `DOCS_SITE` in the [`Makefile`](https://github.com/psyb0t/essessey/blob/main/Makefile). Every page needs front matter with a unique `title` and a `description` of 50 to 170 characters. Link between pages with the relative `.md` path, and to repository files with the full GitHub URL.
