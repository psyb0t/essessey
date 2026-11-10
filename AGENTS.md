# essessey

Instructions for coding agents (Claude Code, Codex and the like) and for anyone else changing this repository. It describes the code as it is; read it before editing.

## Agent setup

`CLAUDE.md` is a committed symlink to `AGENTS.md`, and `.claude` is a committed symlink to `.agents`, so Claude Code reads the same instructions, rules and skills as Codex. Claude's Edit and Write tools refuse to write through a symlink, so edit `AGENTS.md` and `.agents/rules/*.md` directly.

## What it is

`github.com/psyb0t/essessey` is a Go library that streams an LLM turn to a client. A turn is a sequence of `Event` values (an optional id, an event name and a JSON payload) that follows a block protocol: `message_start`, `ping`, indexed `content_block_start`/`content_block_delta`/`content_block_stop` blocks for text, thinking, tool use and tool results, then `message_delta` and `message_stop`. A `Sink` delivers events and a `Source` reads them back.

It ships no binary and no Docker image. It does not call a model ([elelem](https://github.com/psyb0t/elelem) does), and it owns no HTTP handler and no broker connection: callers pass in the writer or client they already have.

## Architecture

- Core package (repository root): `Event`, `Sink`, `Source` and `ErrNoMoreEvents` in `event.go`; the protocol payload types and constants in `types.go`; `Publisher` in `publisher.go`, with one `Send*` method per protocol event plus `SendStreamPreamble`/`SendStreamEpilogue` and the composite `SendToolUseBlock`/`SendToolResultBlock`; `TextStreamer`, `NewThinkingStreamer` and `LineStreamer` in `streamer.go`, which open blocks lazily and own the block index; `Reassemble` in `reassemble.go`, which drains a `Source` into a `ParsedStream`; `InMemorySink` and `SliceSource` in `memory.go`; `MultiSink` in `multisink.go`; `EventStore` and `InMemoryEventStore` in `store.go`.
- `sse/`: the only package that frames. `FrameLines`, `FrameComment` and `FrameRetry` render the WHATWG `text/event-stream` format, `WriterSink` and `HTTPSink` write it, and `Source` parses it back, tracking `LastEventID` and `Retry`.
- `nats/`: `Sink` publishes `Event.Data` raw to `<prefix>.<event type>` through a one-method `Publisher` interface; `Source` is a buffered queue fed by `Deliver`.
- `ws/`: `Sink` writes the whole `Event` with one `WriteJSON` call through a one-method `Conn` interface, under its own mutex; `Source` is the same queue shape as the NATS one.
- `elelemstream/`: `Adapter` binds elelem's `On*` callbacks and turns them into indexed blocks through a `Publisher`. `MapStopReason` maps an elelem finish reason onto a `StopReason`. It is the only package that imports elelem.

Data flow, producing: caller -> `TextStreamer`/`Publisher` (or `elelemstream.Adapter`) -> `Publisher.Publish` (JSON marshal, one DEBUG log line) -> `Sink.Emit` -> SSE bytes, a NATS message or a WebSocket frame. Consuming: SSE bytes, a NATS callback or a WebSocket read loop -> `Source.Next` -> `Reassemble` -> `ParsedStream`.

## Stack

- Go: `go 1.26.0` minimum in `go.mod`; the dev image pins `golang:1.26.6-alpine` by digest in `Dockerfile.dev`.
- Runtime dependencies: `github.com/psyb0t/ctxerrors` (error wrapping), `github.com/psyb0t/ctxscope` (context logger), `github.com/psyb0t/elelem` (`elelemstream` only). Tests use testify.
- golangci-lint v2 is a `tool` dependency in `go.mod`, run as `go tool golangci-lint`. Dependencies are vendored in `vendor/`, and the vendor tree is committed.
- Security scan: govulncheck v1.6.0 and semgrep 1.173.0, both installed in `Dockerfile.dev`.

## Layout

| Path | What lives there |
| --- | --- |
| `event.go`, `types.go` | `Event`, `Sink`, `Source`, `ErrNoMoreEvents`, the payload structs and the `EventType`/`ContentBlockType`/`StopReason`/`Role`/`TimelineItemKind` constants |
| `publisher.go` | `Publisher`, the `Send*` methods, payload-length-only DEBUG logging |
| `streamer.go` | `TextStreamer`, `NewThinkingStreamer`, `LineStreamer`, `LineTransform` |
| `reassemble.go` | `Reassemble`, `ParsedStream` |
| `memory.go`, `multisink.go`, `store.go` | `InMemorySink`, `SliceSource`, `MultiSink`, `EventStore`, `InMemoryEventStore`, `ErrInvalidCapacity` |
| `doc.go` | the core package's doc comment |
| `sse/` | SSE codec (`codec.go`), sinks (`sink.go`), parser (`source.go`), `ErrNotAFlusher`, and the format conformance suite (`conformance_test.go`) |
| `nats/`, `ws/` | the message-oriented bindings |
| `elelemstream/` | `Adapter`, `roundStream`, `MapStopReason`, its two sentinels |
| `scripts/` | `test-coverage.sh` (race-enabled coverage gate) and `sec.sh` (govulncheck plus semgrep, merged into `sec.sarif`) |
| `Dockerfile.dev` | the development and CI image every Make target runs in |
| `docs/` | the documentation site, published by GitHub Pages from `main` at https://psyb0t.github.io/essessey/ |
| `.github/workflows/pipeline.yml` | CI: lint, tests and security scan on every push, release and badges on tags |
| `vendor/` | committed vendored dependencies; never edit by hand |

`docs/` is a Jekyll site with the just-the-docs theme. The package READMEs (`sse/`, `nats/`, `ws/`, `elelemstream/`) are short pointers to their pages. `make docs` writes the built site to `DOCS_SITE` in the `Makefile`, which is gitignored.

## Commands

Docker and Make are enough. Every Go target runs inside the `Dockerfile.dev` image (built by the `dev-image` target, which the others depend on), with the checkout mounted at `/app`.

```bash
make help             # every target
make dep              # go mod tidy and go mod vendor
make lint             # go fix -diff (fails on any diff), then golangci-lint
make lint-fix         # go fix and golangci-lint --fix
make test             # go test -race ./...
make test-coverage    # race tests with coverage, fails below MIN_TEST_COVERAGE (90)
make sec              # govulncheck and semgrep, writes sec.sarif, fails on any finding
make docs             # build the documentation site locally (gitignored output)
make clean            # remove coverage and SARIF artifacts
```

CI runs `make lint`, `make test-coverage` and `make sec` through the reusable `code-workflow.yml`. A `v*` tag waits for the `main` run of the same commit, then cuts the GitHub release and refreshes the badges.

## Rules

Rules for every change live in `.agents/rules/`, one file per topic. Read the ones that apply before editing.
