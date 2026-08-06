# ws — send events over a WebSocket

A `Sink` that writes each `essessey.Event` as one JSON frame, and a `Source`
that turns a read loop back into an `essessey.Source`.

A WebSocket already delivers discrete frames, so nothing here frames anything —
that is the `sse` package's job and only its job.

## Contents

- [Sending](#sending)
- [Receiving](#receiving)
- [What the client actually gets](#what-the-client-actually-gets)
- [Concurrency — read this one](#concurrency--read-this-one)

## Sending

This package never imports a WebSocket library. It asks for one method:

```go
type Conn interface {
	WriteJSON(v any) error
}
```

gorilla's `*websocket.Conn` satisfies it as-is, so you pass your own connection:

```go
func handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sink := ws.NewSink(conn)
	pub := essessey.NewPublisher(r.Context(), sink)

	_ = pub.SendStreamPreamble("msg_1", "conv_1", "some-model")

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

## Receiving

`Source` is a queue with an `essessey.Source` face. Decode frames in your read
loop and hand them to `Deliver`:

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

`Close` stops the source; `Next` drains what is already buffered and then
returns `essessey.ErrNoMoreEvents`. Delivering after `Close` is a no-op that
warns rather than panicking, because a read loop can still be in flight when
you tear down.

## What the client actually gets

One JSON object per frame — the whole `Event`, envelope included:

```json
{"id":"42","event":"content_block_delta","data":{"text":"hi"}}
```

This is the least work of any binding: nothing to parse out of a subject, no
framing to scan. `id` is omitted entirely when empty rather than sent as `""`,
which matters because in the SSE format an empty id RESETS a client's resume
point — keeping the two bindings' semantics aligned means a client can treat a
missing id the same way on both.

## Concurrency — read this one

**gorilla permits exactly one concurrent writer, and this package cannot enforce
that for you.**

`Sink.Emit` takes its own lock, so concurrent `Emit` calls are safe. What is
**not** safe is your application writing to the same connection behind the
sink's back — a ping, a control frame, a close message — while a stream is in
flight. That is a data race gorilla will not protect you from and this package
cannot see.

If anything else writes to that connection, put your own mutex around every
writer including the sink, or funnel all writes through a single goroutine. The
minimal `Conn` interface is what keeps this package free of a WebSocket
dependency, and the cost of that choice is that the locking discipline for
non-essessey writes stays yours.
