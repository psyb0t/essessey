---
title: "SSE binding"
description: "The essessey sse package: WriterSink and HTTPSink, the headers your handler must set, exact wire bytes, the parser, keep-alives, retry and resume."
parent: "Events and bindings"
nav_order: 1
---

# SSE binding

`github.com/psyb0t/essessey/sse` is the only place in essessey that knows what bytes look like. It frames `Event` values into `text/event-stream` and parses them back. The code is in [`sse/`](https://github.com/psyb0t/essessey/tree/main/sse).

It implements the format defined by the WHATWG HTML Living Standard under "Server-sent events", including the parts a single-consumer implementation is tempted to skip.

## Why only this package frames

An HTTP response body is one undelimited byte stream. Nothing in it says where one event stops and the next begins, so the format invents a delimiter: a blank line. NATS and WebSocket already deliver discrete messages, so they need none of this. That's why `nats` and `ws` carry `Event.Data` as it is and only `sse` owns a codec.

## Writing a stream

Two sinks, same framing. `WriterSink` targets any `io.Writer`. `HTTPSink` targets a flushing `http.ResponseWriter` and flushes after every event, so the client sees it right away instead of at the end. Both take a lock, so concurrent `Emit` calls are safe.

```go
func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	sink, err := sse.NewHTTPSink(w)
	if err != nil {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)

		return
	}

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

`NewHTTPSink` returns an error wrapping `sse.ErrNotAFlusher` when the writer is not an `http.Flusher`. That's deliberate: a non-flushing writer buffers the entire response, so the stream arrives as one lump at the end. That looks like a slow model rather than a wiring bug, and would otherwise survive to production.

To write somewhere that isn't HTTP (a file, a buffer, a pipe) use `NewWriterSink(w)`. It takes the same events and produces the same bytes.

## The HTTP headers you must set

**This package does not set them, and your stream will not work without them.**

`Content-Type: text/event-stream` is not advisory: a browser `EventSource` fails the connection outright if the response carries anything else. `Cache-Control: no-store` stops an intermediary from buffering or replaying the stream.

They are the handler's job because the handler owns the response, and essessey does not own your routes. Set them before you build the sink: after the first `Emit` the headers are already on the wire.

## What the bytes look like

`FrameLines` renders one event. This is the whole wire format:

```go
sse.FrameLines(essessey.Event{
	ID:    "42",
	Event: "content_block_delta",
	Data:  json.RawMessage(`{"text":"hi"}`),
})
```

```text
id: 42
event: content_block_delta
data: {"text":"hi"}

```

The trailing blank line ends the event; that's what tells a receiver the event is complete. The `id:` line is written only when `ID` is not empty, and the `event:` line only when `Event` is not empty.

**A multi-line payload becomes one `data:` field per line.** The format has no escaping and no length prefix, so a raw newline inside a value would end the field, and a raw blank line would end the event:

```go
sse.FrameLines(essessey.Event{
	Event: "chunk",
	Data:  json.RawMessage("{\n  \"a\": 1\n}"),
})
```

```text
event: chunk
data: {
data:   "a": 1
data: }

```

The receiver joins them back with newlines, so the payload arrives byte-identical. Writing that as a single `data:` line would put different bytes on the wire than you passed, and for a payload containing a blank line it would forge an extra event out of the remainder. CRLF and lone CR inside a payload are split the same way, and come back as LF.

An empty payload still gets one empty `data:` line, so the event arrives empty instead of vanishing.

CR, LF and NUL are stripped from `ID` and `Event` before framing, so a caller-supplied event type can never inject extra fields or events into the stream.

## Reading a stream

`Source` parses framed bytes back into events. It works against anything that reads: an HTTP response body, a file, a `strings.Reader` in a test.

```go
src := sse.NewSource(resp.Body)

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

Or hand the whole `Source` to [`essessey.Reassemble`](reassembly.md) and get the finished turn instead of handling events yourself.

The parser accepts what the format allows, not just what this package writes:

- The space after a field's colon is optional, so `data:x` and `data: x` are the same value. Only one space is removed, so a payload that starts with spaces keeps them.
- Lines end with CRLF, LF **or** a lone CR.
- A leading byte order mark is stripped.
- Comment lines (starting with `:`) are skipped, and unknown fields are ignored.
- A line with no colon is a field name with an empty value.
- An event with no `data:` field at all is discarded, not delivered empty.
- An `id` containing NUL is ignored and the previous id stays. A `retry` that is not all ASCII digits, or does not fit in an int64, is ignored.

Each event `Next` returns carries the stream's current last event id in `ID`, even when its own frame had no `id:` line.

A partial event at the end of input (no closing blank line) is dropped and logged, because its payload may be incomplete. A single line longer than 1 MiB makes `Next` return a scan error instead of growing without bound.

## Keep-alives and reconnection

```go
io.WriteString(w, sse.FrameComment("keep-alive")) // ": keep-alive\n"
io.WriteString(w, sse.FrameRetry(5*time.Second))  // "retry: 5000\n\n"
```

A comment is a line the receiver ignores. Its use is keeping an idle connection alive: an intermediary that sees no bytes for long enough will drop the connection, and a comment is the cheapest traffic that prevents it. Send one every 15 seconds or so on a stream that can go quiet.

`retry` tells the client how long to wait before reconnecting. It describes the stream rather than any one event, which is why it is its own frame instead of a field on `Event`. A negative duration is written as `0`.

These two helpers return strings; they do not go through a `Sink`. With `HTTPSink`, write them to the same `ResponseWriter`, keep them from interleaving with a concurrent `Emit`, and call `Flush()` on the writer yourself, since only `Emit` flushes. `Publisher.SendPing` is the protocol-level alternative: a `ping` event that goes through the sink like any other.

## Resuming

`Source` tracks the stream-level state a reconnecting client needs:

```go
src.LastEventID() // the last id seen; persists across events that carry none
src.Retry()       // the reconnection delay the server asked for, or 0
```

`LastEventID` is what a client sends back as the `Last-Event-ID` request header, so a server can resume instead of restarting. The server half is on [Resuming a dropped stream](resume.md).

## Things that will bite you

- **No `Content-Type: text/event-stream` means no stream.** This is the most common wiring failure, and nothing in this package can catch it for you.
- **An empty `id:` is not "no id"; it resets the client's resume point.** That is why `Event.ID` is `omitempty`.
- **A frame with an `id:` but no `data:` still advances the client's resume point.** The format sets the last event id before it discards an event with no data, so a bare id frame moves the client without delivering anything. Do not use them as bookmarks.
- **An event with no `data:` field at all is discarded.** A `data:` field with an empty value is different: that dispatches with an empty payload.
- **A payload's own newlines are handled for you, but only through `FrameLines`.** Writing `data:` lines by hand brings back every problem the codec exists to solve.
