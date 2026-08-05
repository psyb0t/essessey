# Changelog

All notable changes per release. Versions follow [semver](https://semver.org)
pre-1.0 conventions: minor bumps may include breaking API changes (called out
explicitly), patch bumps are docs / build / fixes only.

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
