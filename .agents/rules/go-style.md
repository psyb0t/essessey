# Go style

Prefix: GO. `.golangci.yml` is the source of truth; `make lint` must pass before a change is done.

- **GO1:** Lines are at most 80 columns with a tab width of 4 (`lll`). Break long signatures one parameter per line, as `publisher.go` does.
- **GO2:** `wsl_v5` controls blank lines: a blank line before `return` and around blocks, with `branch-max-lines: 2`. `make lint-fix` applies most of it.
- **GO3:** Formatting is `gofumpt`, `goimports` and `gci`. `make lint` also fails when `go fix -diff ./...` prints anything; `make lint-fix` applies it.
- **GO4:** Every `//nolint` names the linter and gives a reason on the same line (`//nolint:containedctx // per-request publisher; ctx bounds the stream`). `nolintlint` reports a directive that no longer suppresses anything.
- **GO5:** No magic numbers (`mnd`): name a constant, as `maxLineSize`, `sourceBufferSize` and `contentBlocksPerTool` do. No package-level variables other than sentinels and compile-time interface checks (`gochecknoglobals`).
- **GO6:** Doc comments say why a type or behavior exists, not just what it is, and end with a period (`godot`).
- **GO7:** Protocol string types (`EventType`, `ContentBlockType`, `StopReason`, `Role`, `TimelineItemKind`) are aliases of `string`, not defined types, so a producer can emit a value this package does not know. Keep new ones as aliases.
- **GO8:** JSON tags on wire types are snake_case to match the protocol. `types.go` carries a file-level `//nolint:tagliatelle` for that; a new wire struct elsewhere needs the same.
