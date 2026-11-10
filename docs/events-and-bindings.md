---
title: "Events and bindings"
description: "The essessey wire model: one Event type, the Sink and Source interfaces, why only SSE frames, and how each binding carries the envelope."
nav_order: 3
has_children: true
---

# Events and bindings

Everybody lists SSE next to WebSocket and NATS like they're three flavors of the same damn thing. They're not. This page is the model the rest of essessey is built on: one event type, two interfaces, and bindings that differ only in how they deliver.

## Event

```go
type Event struct {
	ID    string          `json:"id,omitempty"`
	Event EventType       `json:"event"`
	Data  json.RawMessage `json:"data"`
}
```

That's the whole wire model: an optional id, a name and a JSON payload. It is defined in [`event.go`](https://github.com/psyb0t/essessey/blob/main/event.go). The names and payload shapes the `Publisher` emits are listed on [Event protocol](protocol.md).

`ID` is what makes a dropped connection recoverable. A browser `EventSource` remembers the last id it saw and sends it back as `Last-Event-ID` on reconnect, so a server can resume instead of restarting, and a subscriber that tracks ids can tell it missed one instead of silently rendering a gap. It is `omitempty` on purpose: an empty `id:` field on the SSE wire does not mean "no id", it resets the receiver's resume point, so writing one for an event that has no id would throw away the position every earlier event set up.

The `Publisher` never sets `ID`. If you want ids, stamp them in a `Sink` of your own; [Resuming a dropped stream](resume.md#giving-events-ids) shows one.

## Sink and Source

```go
type Sink interface {
	Emit(ctx context.Context, ev Event) error
}

type Source interface {
	Next(ctx context.Context) (Event, error)
}
```

A `Sink` delivers an event. `Emit` must be safe for concurrent use, because a tool loop can produce blocks from several goroutines; every sink in this module either takes a lock or holds no mutable state.

A `Source` reads events back. `Next` returns `ErrNoMoreEvents` when the stream ends normally. That is the terminator, not a failure, so a caller can loop over a `Source` without special-casing it. Any other error is a real failure.

```go
for {
	ev, err := src.Next(ctx)
	if errors.Is(err, essessey.ErrNoMoreEvents) {
		break
	}

	if err != nil {
		return err
	}

	fmt.Println(ev.Event, string(ev.Data))
}
```

The read side is an interface rather than an `io.Reader` because NATS and WebSocket hand over discrete events with no byte stream to scan. Both shapes satisfy `Next`, so [`Reassemble`](reassembly.md) works against any delivery.

Neither interface knows what framing means. That's a property of the binding underneath, not of the event.

## Who frames and why

| Binding | Needs framing? | Why |
| --- | --- | --- |
| SSE (`io.Writer`, `http.ResponseWriter`) | yes | an HTTP response body is an undelimited byte stream; something has to mark where one event ends |
| NATS | no | every publish is already a discrete message |
| WebSocket | no | every write is already a discrete frame |

So the [SSE binding](sse.md) owns a codec the other two never need, and it implements the format as specified rather than the subset one consumer happens to use.

## What the receiver gets

The `Data` payload is byte-identical on every binding. The envelope around it takes a different route on each:

| Binding | How the envelope arrives |
| --- | --- |
| SSE | `id:`, `event:` and `data:` as wire fields |
| WebSocket | the whole `Event` as one JSON object: `id`, `event` and `data` inline |
| NATS | the payload raw, the event type in the subject, and no id at all |

A NATS subscriber has the most work to do: it rebuilds the event type from the subject it matched. See [NATS binding](nats.md#the-envelope).

## No transport dependencies

`sse` needs nothing beyond the standard library. `nats` and `ws` each declare the one method they need from a client:

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

`*nats.Conn` from `github.com/nats-io/nats.go` and gorilla's `*websocket.Conn` already satisfy these as they are. You pass your own client in, and essessey never imports either SDK, which keeps your module graph clear of the vendor avalanche a real transport client drags in, all for one method per binding. The `depguard` rule in [`.golangci.yml`](https://github.com/psyb0t/essessey/blob/main/.golangci.yml) keeps it that way: only the standard library, testify and `github.com/psyb0t` modules are allowed.

The only third-party imports at runtime are [ctxerrors](https://github.com/psyb0t/ctxerrors) for error wrapping and [ctxscope](https://github.com/psyb0t/ctxscope) for the context logger, plus elelem inside `elelemstream` alone.

## Writing your own Sink or Source

Anything with an `Emit` method is a `Sink`, and anything with a `Next` method is a `Source`. A sink for a different broker, a log, or a test spy needs nothing from this module beyond the `Event` type. Two rules keep it consistent with the built-in ones:

- Make `Emit` safe for concurrent calls.
- Return `essessey.ErrNoMoreEvents` from `Next` when the input ends cleanly, and wrap it if you add context, so `errors.Is` still matches.

The module ships two in-process implementations that are handy beyond tests, in [`memory.go`](https://github.com/psyb0t/essessey/blob/main/memory.go):

- `InMemorySink` collects events instead of delivering them. `Events()` returns a copy and `Len()` the count. It is also what you want when a turn has to be fully produced before any of it is released, such as a moderation pass.
- `SliceSource` replays a fixed slice (it copies the slice at construction). Feeding one `InMemorySink`'s `Events()` into a `SliceSource` round-trips a whole stream with no transport involved.

`MultiSink` fans one `Emit` out to several sinks; it is covered on [Resuming a dropped stream](resume.md#multisink).
