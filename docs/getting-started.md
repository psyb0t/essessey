---
title: "Getting started"
description: "Install essessey, publish a first LLM turn into an in-memory sink, swap in a real delivery, and reassemble the stream on the receiving side."
nav_order: 2
---

# Getting started

The shortest path from nothing to a working stream and back. It uses `InMemorySink`, which needs no server, no broker and no network, then shows what changes when you point it at a real delivery.

## Install

```bash
go get github.com/psyb0t/essessey
```

You need Go 1.26 or newer.

## Emit a turn

A `Publisher` writes protocol events to a `Sink`. A `TextStreamer` turns a chunk-at-a-time answer into one correctly indexed text block. `InMemorySink` collects whatever got emitted:

```go
package main

import (
	"context"
	"fmt"

	"github.com/psyb0t/essessey"
)

func main() {
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
}
```

That prints `8 events emitted`: `message_start` and `ping` from the preamble, one `content_block_start`, two `content_block_delta`, one `content_block_stop`, then `message_delta` and `message_stop` from the epilogue. [Event protocol](protocol.md) shows each payload.

## Swap the delivery

Replace the sink and nothing above it changes:

| Sink | Delivers to |
| --- | --- |
| `sse.NewWriterSink(w)` | any `io.Writer`, as `text/event-stream` bytes |
| `sse.NewHTTPSink(w)` | a flushing `http.ResponseWriter`, flushed after every event |
| `nats.NewSink(conn, subjectPrefix)` | NATS subjects `<subjectPrefix>.<event type>` |
| `ws.NewSink(conn)` | a WebSocket, one JSON object per frame |
| `essessey.NewMultiSink(a, b)` | several sinks at once |

The `Publisher`, the streamer and the event sequence have no idea which delivery is on the other end, and no reason to. Each binding has its own page: [SSE](sse.md), [NATS](nats.md), [WebSocket](websocket.md).

## Read it back

`Reassemble` drains any `Source` and hands back the finished turn. `SliceSource` replays a slice of events, so an `InMemorySink` round-trips with no transport at all:

```go
parsed := essessey.Reassemble(ctx, essessey.NewSliceSource(sink.Events()))

fmt.Println(parsed.StreamID) // conv_1
fmt.Println(parsed.Text)     // Hello, world!
```

Against a real delivery you swap the source the same way you swapped the sink, for example `sse.NewSource(resp.Body)` on an HTTP client. [Reading a stream back](reassembly.md) covers the rest of `ParsedStream`.

## Next

- Stream reasoning, tool calls and line-validated output: [Publisher and streamers](publishing.md).
- Drive it from elelem without writing the index arithmetic: [elelem adapter](elelemstream.md).
- Let a client reconnect mid-stream: [Resuming a dropped stream](resume.md).
