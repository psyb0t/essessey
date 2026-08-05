# essessey

[![Go Reference](https://pkg.go.dev/badge/github.com/psyb0t/essessey.svg)](https://pkg.go.dev/github.com/psyb0t/essessey)
[![CI](https://github.com/psyb0t/essessey/actions/workflows/pipeline.yml/badge.svg?branch=main)](https://github.com/psyb0t/essessey/actions/workflows/pipeline.yml)
[![coverage](https://raw.githubusercontent.com/psyb0t/essessey/badges/coverage.svg)](https://github.com/psyb0t/essessey/actions/workflows/pipeline.yml)
[![version](https://raw.githubusercontent.com/psyb0t/essessey/badges/version.svg)](https://github.com/psyb0t/essessey/tags)
[![license](https://raw.githubusercontent.com/psyb0t/essessey/badges/license.svg)](LICENSE)

Say the letters. That's the name.

Getting a model's answer to whoever is waiting for it — token by token, block
by block, in the order it actually happened.

Here is the hill this package dies on: **SSE is a format, not a transport.**
Everybody lists it next to WebSocket and NATS as though they were three ways
of doing the same thing. They are not. `event:` / `data:` / blank line exists
for exactly one reason — an HTTP response body is a pipe with no seams, so
something has to mark where one event stops and the next starts. Give the
same events to NATS or a WebSocket and that framing is dead weight: those
already deliver discrete messages. So here SSE is one binding that adds
framing, and the message-oriented ones don't, and all of them carry the same
`Event`. A browser `EventSource` and a NATS subscriber parse identical JSON.

It does not talk to a model — that is [elelem](https://github.com/psyb0t/elelem)'s
job, and `elelemstream` is the seam between them. It does not own your HTTP
handler, does not pick your broker, and does not drag a client library into
your build to prove it supports one: the NATS and WebSocket bindings are
written against the smallest interface each needs, so you hand them the
connection you already have and this package stays at zero transport
dependencies.

What it does own is the boring part nobody wants to write twice — which
content block index a tool result belongs to, when a thinking block has to
close before the answer starts, and putting a stream back together at the
other end.

```go
sink := essessey.NewInMemorySink() // or sse.NewWriterSink(w), nats.NewSink(conn, "turn"), ws.NewSink(conn)
pub := essessey.NewPublisher(ctx, sink)

if err := pub.SendStreamPreamble(msgID, conversationID, model); err != nil {
	return err
}

streamer := essessey.NewTextStreamer(pub, 0)
for chunk := range delta {
	if err := streamer.Write(ctx, chunk); err != nil {
		return err
	}
}

if err := streamer.Close(ctx); err != nil {
	return err
}

return pub.SendStreamEpilogue(essessey.StopReasonEndTurn, outputTokens)
```

## Contents

- [Quick start](#quick-start)
- [Why one Event, many bindings](#why-one-event-many-bindings)
- [What each package does](#what-each-package-does)
- [Zero transport dependencies](#zero-transport-dependencies)
- [elelemstream](#elelemstream)
- [Layout](#layout)
- [Development](#development)
- [License](#license)

## Quick start

```bash
go get github.com/psyb0t/essessey
```

A `Publisher` writes to any `Sink`. `InMemorySink` needs nothing to try —
it just collects what was emitted:

```go
ctx := context.Background()

sink := essessey.NewInMemorySink()
pub := essessey.NewPublisher(ctx, sink)

if err := pub.SendStreamPreamble("msg_1", "conv_1", "some-model"); err != nil {
	panic(err)
}

streamer := essessey.NewTextStreamer(pub, 0)

if err := streamer.Write(ctx, "Hello, "); err != nil {
	panic(err)
}

if err := streamer.Write(ctx, "world!"); err != nil {
	panic(err)
}

if err := streamer.Close(ctx); err != nil {
	panic(err)
}

if err := pub.SendStreamEpilogue(essessey.StopReasonEndTurn, 0); err != nil {
	panic(err)
}

fmt.Println(sink.Len(), "events emitted")
```

Swap `InMemorySink` for `sse.NewWriterSink(w)` (or `sse.NewHTTPSink(w)` behind
a flushing `http.ResponseWriter`), `nats.NewSink(conn, subjectPrefix)`, or
`ws.NewSink(conn)` and every line above the sink construction is unchanged —
the `Publisher`, the streamer, and the event sequence don't know or care
which delivery is on the other end.

## Why one Event, many bindings

```go
type Event struct {
	Event EventType       `json:"event"`
	Data  json.RawMessage `json:"data"`
}
```

That's the whole wire model: a name and a JSON payload. `Sink` delivers it
(`Emit(ctx, Event) error`); `Source` reads it back (`Next(ctx) (Event,
error)`, ending the stream with `ErrNoMoreEvents`). Neither interface knows
what "framing" means — that's a property of the binding underneath, not of
the event.

| binding | needs framing? | why |
|---|---|---|
| SSE (`io.Writer`, `http.ResponseWriter`) | yes | an HTTP response body is an undelimited byte stream; something has to mark where one event ends |
| NATS | no | every publish is already a discrete message |
| WebSocket | no | every write is already a discrete frame |

So the SSE binding owns a codec (`event:` / `data:` lines plus the blank-line
terminator) that the other two never need. What travels as `Data` is the
same `json.RawMessage` regardless — a message published to NATS and a chunk
scanned off an SSE byte stream decode into the identical Go struct on the
receiving end.

## What each package does

| Package | Responsibility |
|---|---|
| **Core** (this package) | `Event`, the `Sink`/`Source` interfaces, `Publisher` (one `Send*` method per protocol event, plus `SendStreamPreamble`/`SendStreamEpilogue` for the open/close pair), `TextStreamer`/`LineStreamer` for turning a chunk-at-a-time answer into correctly-indexed content blocks, and `Reassemble`, which drains a `Source` back into a `ParsedStream` — accumulated text, tool calls matched to their results by content-block index, and an ordered timeline of both. |
| **[sse](sse/)** | The SSE format itself: `FrameLines` renders the wire bytes, `WriterSink`/`HTTPSink` write framed events to an `io.Writer` or a flushing `http.ResponseWriter`, and `Source` scans them back off an `io.Reader` — a malformed frame is warn-logged and skipped rather than aborting the stream. |
| **[nats](nats/)** | A `Sink` that publishes `Event.Data` unframed to `subjectPrefix.<eventType>`, and a `Source` whose `Deliver` method is wired as a subscription callback. |
| **[ws](ws/)** | A `Sink` that writes the whole `Event` as one `WriteJSON` call, and a `Source` whose `Deliver` method is wired into a read loop. |
| **[elelemstream](elelemstream/)** | Bridges [elelem](https://github.com/psyb0t/elelem)'s callbacks to this protocol — see below. |
| Test doubles (`memory.go`) | `InMemorySink` collects events instead of delivering them (not test-only — also what you want when a turn must be fully produced before any of it is released), and `SliceSource` replays a fixed slice, so feeding one `InMemorySink`'s `Events()` into a `SliceSource` round-trips a stream with no transport involved at all. |

## Zero transport dependencies

`sse` needs nothing beyond the standard library. `nats` and `ws` each
declare the one method they actually need from a client:

```go
// nats.Publisher
type Publisher interface {
	Publish(subject string, data []byte) error
}

// ws.Conn
type Conn interface {
	WriteJSON(v any) error
}
```

`*nats.Conn` (`github.com/nats-io/nats.go`) and a gorilla `*websocket.Conn`
already satisfy these as-is — you pass your own client in, and essessey
never imports either SDK. That keeps the module graph out of the `go mod
vendor` cascade a real transport client drags in, for the sake of one method
each binding actually calls.

## elelemstream

`elelemstream` is the one subpackage that imports
[elelem](https://github.com/psyb0t/elelem): it translates elelem's callback
stream (text deltas, reasoning deltas, tool-call starts and tool results)
into this package's block protocol, so an elelem-backed handler gets the
same `message_start` → content blocks → `message_stop` sequence without
hand-rolling the translation. Everything else in this module — the
core package, `sse`, `nats`, `ws` — stays free of an elelem import; a caller
who isn't using elelem never pulls it in.

The block-index arithmetic it owns is the part worth reading before you touch
it — which index a tool result lands on, why parallel calls break naive
implementations, and which invariants the tests pin. That lives next to the
code, in [elelemstream/README.md](elelemstream/README.md).

## Layout

```text
types.go, event.go             the wire types, EventType/Role/etc. constants, Event, Sink, Source
publisher.go                   Publisher and one Send* method per protocol event
streamer.go                    TextStreamer, LineStreamer
reassemble.go                  Source -> ParsedStream reconstruction
memory.go                      InMemorySink, SliceSource
sse/                           the SSE format: codec, WriterSink, HTTPSink, Source
nats/                          NATS binding over a minimal Publisher interface
ws/                            WebSocket binding over a minimal Conn interface
elelemstream/                  elelem callbacks -> block protocol (imports elelem)
```

## Development

```bash
make dep           # tidy the module and re-vendor
make lint          # go fix, then golangci-lint at full strictness
make lint-fix      # the same, applying what it can fix itself
make test          # the suite, always with -race
make test-coverage # the suite plus the coverage floor CI enforces
make help          # the rest
```

## License

MIT. See [LICENSE](LICENSE).

See [CHANGELOG.md](CHANGELOG.md) for release notes.
