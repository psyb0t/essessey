# Changelog

All notable changes per release. Versions follow [semver](https://semver.org)
pre-1.0 conventions: minor bumps may include breaking API changes (called out
explicitly), patch bumps are docs / build / fixes only.

## v0.4.1 — 2026-08-05

README only. No code, no API, no behaviour change.

- The README now reads like the rest of the psyb0t libraries instead of a
  whitepaper. Same facts, same structure, same links, same hill it dies on
  (SSE is a format, not a transport) — just in the voice the rest of the
  ecosystem already uses.

## v0.4.0 — 2026-08-05

Tracks [elelem](https://github.com/psyb0t/elelem) v0.4.0. No API change here.

- **Requires elelem v0.4.0 or later**, and this is the breaking part: upgrading
  essessey pulls elelem v0.4.0 into your build, where `Request.Complete`,
  `Request.Stream` and `Request.CompleteInto` no longer exist. Migration is
  mechanical — `Complete(ctx)` becomes `Run(ctx)`, `CompleteInto(ctx, &v)`
  becomes `RunInto(ctx, &v)`, and `Stream(ctx, fn)` becomes
  `OnDelta(fn).Run(ctx)`. See elelem's changelog for the one behaviour change
  to check for.

- Nothing in this package moved. `Adapter`, `Bind` and every exported callback
  have the same signatures and the same block output.

- `elelem.WithStreaming(false)` — new upstream, for backends that cannot serve
  a streaming call — is transparent here. elelem feeds the finished response
  through the same callbacks the `Adapter` binds, so the blocks on the wire are
  identical; a subscriber only sees them arrive together at the end of the turn
  rather than filling in. Documented in
  [elelemstream/README.md](elelemstream/README.md).

## v0.3.0 — 2026-08-05

`Bind` composes with an app's own callbacks, because the reason it could not
was fixed upstream rather than worked around here.

- **Requires [elelem](https://github.com/psyb0t/elelem) v0.3.0 or later.** Its
  `On*` setters now append to a chain instead of replacing what was
  registered, so an app can call `Bind` and then register its own
  `OnRoundStart` and both run:

  ```go
  adapter.Bind(req).
      OnRoundStart(func(context.Context, *elelem.RoundEvent) error {
          heartbeat.Touch()

          return nil
      })
  ```

  v0.2.0 documented the opposite as a trap to route around. Exporting the
  callbacks made composition possible but still left every caller responsible
  for remembering the ordering rule — a hazard nobody hits until their stream
  silently stops rendering. Fixing the setters upstream removes it for
  everyone instead.

- The exported callbacks stay. They are no longer the workaround, they are the
  way to place a hook at an exact point relative to the `Adapter`'s, or to wire
  only some of them.

- `elelemstream/adapter_test.go` now runs a real turn through a scripted driver
  with an app callback registered after `Bind`, and asserts blocks still reach
  the sink. The old failure mode produced no error at all, so only an
  end-to-end run catches it — verified by removing the `Adapter`'s
  `OnRoundStart` registration and watching the test fail.

## v0.2.0 — 2026-08-05

`elelemstream`'s callbacks are exported, so an app can wrap them.

- **Breaking (in practice, not in signature): `Bind` alone could not serve an
  app with its own per-round concerns.** elelem's `On*` setters REPLACE rather
  than append, so a caller that registered its own `OnRoundStart` after `Bind`
  silently unregistered the adapter's and stopped emitting content blocks
  altogether — no error, just a stream that never renders. Every callback is
  now exported (`OnRoundStart`, `OnDelta`, `OnAssistantMessage`, `OnRoundEnd`,
  `OnToolCallStart`, `OnToolResult`), so an app registers its own hook and
  calls the adapter's from inside it:

  ```go
  req.OnRoundStart(func(ctx context.Context, ev *elelem.RoundEvent) error {
      heartbeat.Touch()
      return adapter.OnRoundStart(ctx, ev)
  })
  ```

  `Bind` stays as the convenience for the case with no extra concerns, and now
  documents the trap rather than leaving it to be discovered.

  Found by doing the first real integration rather than by reading the code —
  the callbacks were unexported, so there was no way to compose at all.

## v0.1.1 — 2026-08-05

Documentation. No API or behaviour change.

- **The core package had no package doc**, so `pkg.go.dev` showed a bare
  symbol list for the package a reader lands on first. Added `doc.go`
  covering the `Event` model, why SSE is treated as a format rather than a
  transport peer, and the lazy-open / index-advance rules that the block
  protocol depends on.
- Added `elelemstream/README.md`. The block-index arithmetic is the one piece
  where being slightly wrong fails silently — a tool result renders into the
  wrong card rather than erroring — so the layout table, the invariants, and
  why parallel tool calls break naive implementations now live next to that
  code instead of only in its tests.
- Reworded the top-level README. Several headings and one table header had
  been carried over verbatim from a sibling project's README rather than
  written for this one.

## v0.1.0 — 2026-08-05

First release. One `Event` — a name plus a JSON payload — streamed to a
client over whichever delivery the caller has, with the wire payload
identical across all of them.

- Core protocol: `Event`, the `Sink`/`Source` interfaces, and `Publisher`
  with one `Send*` method per protocol event (`message_start`,
  `content_block_start/delta/stop`, `message_delta`, `message_stop`, plus
  text, thinking, tool-use and tool-result variants) and the
  `SendStreamPreamble`/`SendStreamEpilogue` pair for opening and closing a
  turn.
- `TextStreamer` and `LineStreamer` accumulate a chunk-at-a-time answer and
  emit correctly-indexed content blocks; both work with a nil `Publisher` for
  non-streaming accumulation.
- `Reassemble` drains a `Source` back into a `ParsedStream`: accumulated
  text, tool calls matched to their results by content-block index, and an
  ordered timeline of both. A malformed or orphaned individual event is
  warn-logged and dropped rather than aborting reconstruction.
- `InMemorySink` and `SliceSource` round-trip a stream with no transport
  involved — feed one's `Events()` into the other.
- `sse`: the SSE format itself — `FrameLines` renders the wire bytes,
  `WriterSink`/`HTTPSink` write framed events to an `io.Writer` or a
  flushing `http.ResponseWriter`, and `Source` scans them back off an
  `io.Reader`.
- `nats` and `ws`: message-oriented bindings that need no framing, each
  defined over the one minimal interface it actually needs from a client
  (`Publish(subject, data)` and `WriteJSON(v)` respectively) rather than
  importing a transport SDK.
- `elelemstream`: bridges [elelem](https://github.com/psyb0t/elelem)'s
  callback stream into this protocol. It is the only subpackage that imports
  elelem — the core package and the `sse`/`nats`/`ws` bindings stay free of
  it.
- Failures are matchable rather than stringly: `ErrNoMoreEvents` ends a
  `Source`, `sse.ErrNotAFlusher` rejects a non-streaming
  `http.ResponseWriter` at construction, and
  `elelemstream.ErrRoundStreamNotInitialized` /
  `elelemstream.ErrToolResultMissing` name the two callback-wiring faults.
  All are wrapped with call-site context, so `errors.Is` works through the
  wrap.
