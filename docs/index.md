---
title: "Home"
description: "essessey is a Go library that streams an LLM turn as one event model over SSE, NATS or WebSocket and reassembles it on the other end."
nav_order: 1
permalink: /
seo:
  type: WebSite
---

# essessey

Say the letters out loud. That's the name. S-S-E.

essessey gets a model's answer to whoever is waiting for it: token by token, block by block, in the order it actually happened. A turn is a sequence of `Event` values (an optional id, a name and a JSON payload). A `Sink` delivers them over SSE, NATS, WebSocket or anything else you write, and a `Source` reads them back so `Reassemble` can glue the turn together again at the other end.

The hill this package dies on: **SSE is a format, not a transport.** The `event:` / `data:` / blank line framing exists because an HTTP response body is a pipe with no seams. NATS and WebSocket already deliver discrete messages, so there the framing is dead weight. Here SSE is one binding that adds framing, the message-oriented ones don't, and all of them carry the same `Event`. [Events and bindings](events-and-bindings.md) has the whole argument.

It doesn't talk to a model; that's [elelem](https://github.com/psyb0t/elelem)'s job, and the [elelem adapter](elelemstream.md) is the seam between them. It doesn't own your HTTP handler and doesn't pick your broker. The NATS and WebSocket bindings are written against the one method each needs, so you hand them the connection you already have and essessey pulls in no transport SDK.

## Install

```bash
go get github.com/psyb0t/essessey
```

The module declares `go 1.26.0` in its [`go.mod`](https://github.com/psyb0t/essessey/blob/main/go.mod), so you need Go 1.26 or newer.

## Quick look

```go
sink := essessey.NewInMemorySink() // or sse.NewWriterSink(w), nats.NewSink(conn, "turn"), ws.NewSink(conn)
pub := essessey.NewPublisher(ctx, sink)

if err := pub.SendStreamPreamble(msgID, streamID, model); err != nil {
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

## Where to go next

| You want to | Read |
| --- | --- |
| Emit a first stream and read it back | [Getting started](getting-started.md) |
| Understand `Event`, `Sink`, `Source` and why SSE is not a transport | [Events and bindings](events-and-bindings.md) |
| See every event, payload and block type on the wire | [Event protocol](protocol.md) |
| Emit text, reasoning, line-checked output and tool blocks | [Publisher and streamers](publishing.md) |
| Turn a stream back into text, tool calls and a timeline | [Reading a stream back](reassembly.md) |
| Let a client reconnect without losing events | [Resuming a dropped stream](resume.md) |
| Stream over HTTP with `text/event-stream` | [SSE binding](sse.md) |
| Publish to NATS subjects | [NATS binding](nats.md) |
| Send events over a WebSocket | [WebSocket binding](websocket.md) |
| Wire elelem's callbacks straight into the protocol | [elelem adapter](elelemstream.md) |
| Build, lint and test the library itself | [Development](development.md) |

The Go package docs are on [pkg.go.dev](https://pkg.go.dev/github.com/psyb0t/essessey), and the source is on [GitHub](https://github.com/psyb0t/essessey).
