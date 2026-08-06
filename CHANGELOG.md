# Changelog

All notable changes per release. Versions follow [semver](https://semver.org)
pre-1.0 conventions: minor bumps may include breaking API changes (called out
explicitly), patch bumps are docs / build / fixes only.

## v0.6.0 — 2026-08-06

Retention and fan-out, so the event ids added in v0.5.0 can actually be used to
resume a dropped stream. Additive — nothing existing changes behaviour.

### Added

- `MultiSink` — fans one `Emit` out to several sinks. Every other sink is
  terminal, so there was previously no way to send the same event to two
  places. It forwards to all of them even when one fails, returning the joined
  error: a full store or a broken audit sink must not cost the client its
  stream, but the failure must not vanish either. A non-nil error therefore
  means "at least one destination missed it", not "nothing was delivered".

- `EventStore` — retains recent events per stream so a reconnecting client can
  be resumed. Its read side reports whether the resume point is KNOWN rather
  than guessing: when an id has aged out or never existed, replaying from the
  start duplicates everything the client already rendered, and replaying from
  now silently drops the events in between — which is the exact gap ids exist
  to prevent. Only the caller can decide, so `Since` hands that decision back.

- `InMemoryEventStore` — a bounded, per-stream, concurrency-safe default. Per
  stream it keeps a ring buffer for order and eviction plus an id index for
  lookup; ids are opaque strings with no ordering, so a ring alone would make
  every resume a linear scan. `SinkFor(streamID)` is its write half, so
  retention composes with `MultiSink` instead of needing a wrapper type.

### Notes

Replay deliberately introduces no new code path: ask the store what came after
the client's last id, wrap it in the existing `SliceSource`, and pump it through
the ordinary `Sink`. A separate replay path is a path that can drift from the
live one, and a test asserts the two are indistinguishable.

Behaviours pinned by tests rather than left implicit:

- Events with no id are retained and replayed, but cannot be resumed TO —
  dropping them would silently skip real content.
- A duplicate id moves the resume point to the later occurrence, skipping what
  came between. Ids are assumed unique per stream.
- Eviction removes the evicted id from the index, so a long-dead id never
  reports as a valid resume point and the index cannot grow without bound.
- A capacity of zero or less is refused at construction — it would accept every
  append and resume nothing, a buffer that silently never works.

Two things the store cannot do for you, documented in the README: replay must go
through the wire sink alone (replaying into the `MultiSink` re-appends what it is
replaying), and `streamID` is a security boundary — replay keyed only by event id
would hand one client another's events.

## v0.5.0 — 2026-08-06

The SSE binding now implements the wire format as specified, instead of the
subset one consumer happened to exercise. **Breaking** for anyone who depended
on the old framing bytes or the old parser's behaviour — details below.

### Fixed

- **The codec could not round-trip its own output.** A sink wrote a payload of
  `{\n"a":1\n}` and the matching source read back `{`: truncated at the first
  newline, remainder discarded, no error. Any payload containing a newline was
  silently corrupted end to end.

- **A payload containing a blank line forged a second event.** The framer wrote
  the whole payload as ONE `data:` line, but the format has no escaping and no
  length prefix — a newline inside a value ends the field and a blank line ends
  the event. Payloads are now emitted as one `data:` field per line, which is
  how the format represents a multi-line value.

- **An event type containing a newline could inject events.** Same missing
  split on the `event:` line: one `Emit` call could put several complete,
  caller-chosen events on the wire. Field values that cannot survive a single
  line (CR, LF, NUL) are now stripped before framing.

- **Conformant streams from other producers were unreadable.** The parser
  matched the literal prefix `"data: "`, space included. That space is OPTIONAL
  in the format, so a producer writing `data:x` yielded nothing at all — no
  event, no error. Field lines are now parsed properly: name before the first
  colon, value after, exactly one leading space removed if present.

- **Multiple `data:` fields lost everything after the first.** They are joined
  with newlines into a single value, which is the only way a multi-line payload
  survives.

- **Events were emitted on the wrong trigger.** The parser paired an `event:`
  line with the next `data:` line; the format ends an event at a BLANK LINE.
  With the state machine in place, a lone `retry:` or a block of comments no
  longer produces a phantom event, and an event with no data field is discarded
  rather than delivered empty.

- **A lone CR was not a line terminator**, so a stream using bare CR — which
  the format permits — parsed as nothing at all.

- **A leading byte order mark swallowed the first event**, because it glued
  itself to the first field name.

- **Valid frames were being dropped as malformed.** A `data:` line with no
  preceding `event:` line is a real event whose type is simply absent, and two
  `event:` lines in a row means the last one wins. Both were previously skipped.

### Added

- `Event.ID` — the event identifier, emitted as `id:` and parsed back. This is
  what makes a dropped connection recoverable: a client returns the last id it
  saw as `Last-Event-ID` on reconnect, so a server can resume rather than
  restart, and a subscriber tracking ids can detect a gap instead of silently
  rendering one. Empty is omitted on purpose — an empty `id:` field RESETS the
  receiver's resume point rather than meaning "no id".
- `Source.LastEventID()` and `Source.Retry()` — the stream-level state a client
  needs to reconnect correctly. The last id persists across events until the
  producer changes it; an id containing NUL is ignored, leaving the previous
  one intact so a malformed value cannot destroy a valid resume point.
- `FrameRetry(time.Duration)` — tells the client how long to wait before
  reconnecting. It describes the stream, not an event, so it is its own frame
  rather than a field on `Event`.
- `FrameComment(string)` — the keep-alive an intermediary needs to see so it
  does not drop an idle connection.

### Changed

- **Breaking:** `FrameLines` output differs for any payload containing a
  newline, and for any event carrying an ID. Byte-for-byte comparisons against
  the old output will fail; the new bytes are what the format actually
  specifies.
- **Breaking:** `Source` now delivers events the old parser dropped (typeless
  events) and no longer delivers phantom ones.
- The README no longer claims a NATS subscriber and an `EventSource` receive
  identical JSON. The PAYLOAD is identical; the envelope is not. WebSocket
  sends the whole `Event` as one object, NATS publishes the payload raw with
  the type in the subject and carries no id, SSE carries all three as wire
  fields. A NATS subscriber has the most reconstruction to do, and that is now
  stated rather than implied away.

## v0.4.2 — 2026-08-05

CI only. No code, no API, no behaviour change.

- **This repo was missing three of the four workflows every other public repo
  here runs.** It had `pipeline.yml` and nothing else, so it was never mirrored,
  never archived, and had no PR gate.
  - `mirror-and-archive.yml` — pushes mirror to GitLab and Codeberg; the
    default branch and tags additionally save to the Wayback Machine.
  - `issue-pull.yml` — relays issues opened on the mirrors back here.
  - `collaborators-only.yml` — closes and locks PRs from non-collaborators.
- The two cron slots are **this repo's own**, not copies of another repo's.
  Every mirrored repo holds a unique monthly archive slot, because the Wayback
  save is rate-limited and takes roughly two minutes; reusing a slot would put
  two repos in it. `issue-pull` staggers its minute for the same reason, since
  GitHub fires an account's crons together.

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
