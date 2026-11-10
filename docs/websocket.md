---
title: "WebSocket binding"
description: "The essessey ws package: write each event as one JSON frame through a one-method Conn, read frames back into a Source, and the one-writer rule."
parent: "Events and bindings"
nav_order: 3
---

# WebSocket binding

`github.com/psyb0t/essessey/ws` has a `Sink` that writes each `Event` as one JSON frame and a `Source` that turns a read loop back into an `essessey.Source`. The code is in [`ws/`](https://github.com/psyb0t/essessey/tree/main/ws).

A WebSocket already delivers discrete frames, so nothing here frames anything. That's the [SSE binding](sse.md)'s job and only its job.

## Sending

This package never imports a WebSocket library. It asks for one method:

```go
type Conn interface {
	WriteJSON(v any) error
}
```

gorilla's `*websocket.Conn` satisfies it as it is, so you pass your own connection:

```go
func handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sink := ws.NewSink(conn)
	pub := essessey.NewPublisher(r.Context(), sink)

	if err := pub.SendStreamPreamble("msg_1", "conv_1", "some-model"); err != nil {
		return
	}

	streamer := essessey.NewTextStreamer(pub, 0)
	for _, chunk := range []string{"Hello, ", "world!"} {
		if err := streamer.Write(r.Context(), chunk); err != nil {
			return
		}
	}

	_ = streamer.Close(r.Context())
	_ = pub.SendStreamEpilogue(essessey.StopReasonEndTurn, 0)
}
```

## What the client gets

One JSON object per frame, the whole `Event`, envelope included:

```json
{
  "id": "42",
  "event": "content_block_delta",
  "data": {
    "type": "content_block_delta",
    "index": 0,
    "delta": {
      "type": "text_delta",
      "text": "hi"
    }
  }
}
```

(Pretty-printed here; on the wire it is one line.) This is the least work of any binding: nothing to parse out of a subject, no framing to scan. `id` is left out entirely when empty rather than sent as `""`. That keeps it in line with SSE, where an empty id resets a client's resume point, so a client can treat a missing id the same way on both. The `Publisher` never sets an id; see [Resuming a dropped stream](resume.md#giving-events-ids) to add them.

## Receiving

`Source` is a queue with an `essessey.Source` face. Decode frames in your read loop and hand them to `Deliver`:

```go
src := ws.NewSource()

go func() {
	defer src.Close()

	for {
		var ev essessey.Event
		if err := conn.ReadJSON(&ev); err != nil {
			return
		}

		src.Deliver(ev)
	}
}()

parsed := essessey.Reassemble(ctx, src)
fmt.Println(parsed.Text)
```

- `Next` blocks until an event arrives, the source is closed and drained, or the context is done (that returns the wrapped context error).
- `Close` stops the source. `Next` drains what is already buffered and then returns `essessey.ErrNoMoreEvents`. `Close` can be called more than once.
- `Deliver` after `Close` is a no-op that logs a warning instead of panicking, because a read loop can still be in flight when you tear down.
- The queue holds 64 events. When it is full, `Deliver` blocks the read loop until `Next` takes one.

## Concurrency: read this one

**gorilla permits exactly one concurrent writer, and this package cannot enforce that for you.**

`Sink.Emit` takes its own lock, so concurrent `Emit` calls are safe. What is **not** safe is your application writing to the same connection behind the sink's back (a ping, a control frame, a close message) while a stream is in flight. That is a data race gorilla will not protect you from and this package cannot see.

If anything else writes to that connection, put your own mutex around every writer including the sink, or send all writes through a single goroutine. The one-method `Conn` interface is what keeps this package free of a WebSocket dependency, and the cost of that choice is that the locking for writes outside essessey stays yours.
