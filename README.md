# essessey

[![Go Reference](https://pkg.go.dev/badge/github.com/psyb0t/essessey.svg)](https://pkg.go.dev/github.com/psyb0t/essessey)
[![CI](https://github.com/psyb0t/essessey/actions/workflows/pipeline.yml/badge.svg?branch=main)](https://github.com/psyb0t/essessey/actions/workflows/pipeline.yml)
[![coverage](https://raw.githubusercontent.com/psyb0t/essessey/badges/coverage.svg)](https://github.com/psyb0t/essessey/actions/workflows/pipeline.yml)
[![version](https://raw.githubusercontent.com/psyb0t/essessey/badges/version.svg)](https://github.com/psyb0t/essessey/tags)
[![license](https://raw.githubusercontent.com/psyb0t/essessey/badges/license.svg)](https://github.com/psyb0t/essessey/blob/main/LICENSE)
[![imported by](https://raw.githubusercontent.com/psyb0t/essessey/badges/importers.svg)](https://github.com/psyb0t/essessey/blob/badges/importers.md)

Say the letters out loud. That's the name. S-S-E.

Getting a model's answer to whoever's waiting for it: token by token, block by block, in the order it actually fucking happened.

Here's the hill this package is prepared to die on: **SSE is a format, not a transport.** Everybody lists it next to WebSocket and NATS like they're three flavors of the same damn thing. They're not. `event:` / `data:` / blank line exists for exactly one reason: an HTTP response body is a pipe with no seams, so something has to mark where one event stops and the next starts. Hand those same events to NATS or a WebSocket and that framing is dead weight, because those already deliver discrete messages. So here SSE is one binding that adds framing, the message-oriented ones don't, and all of them carry the same `Event`. A browser `EventSource` and a NATS subscriber decode the identical JSON payload; only the envelope around it differs per binding. That's the whole fucking point.

It doesn't talk to a model; that's [elelem](https://github.com/psyb0t/elelem)'s job, and `elelemstream` is the seam between them. It doesn't own your HTTP handler, doesn't pick your broker, and doesn't drag half of npm's Go equivalent into your build just to prove it supports one: the NATS and WebSocket bindings are written against the smallest interface each actually needs, so you hand them the connection you already have and this package stays at zero transport dependencies. Zero. Check the [`go.mod`](https://github.com/psyb0t/essessey/blob/main/go.mod) yourself.

What it DOES own is the boring shit nobody wants to write twice: which content block index a tool result belongs to, when a thinking block has to close before the answer starts, and gluing a stream back together at the other end.

## Contents

- [Quick start](#quick-start)
- [What's in the box](#whats-in-the-box)
- [Documentation](#documentation)
- [Development](#development)
- [Sponsor](#sponsor)
- [License](#license)

## Quick start

```bash
go get github.com/psyb0t/essessey
```

A `Publisher` writes to any `Sink`. `InMemorySink` needs nothing at all to try; it just collects whatever got emitted:

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

Swap the sink and every line above it stays exactly the fucking same. The other half, `Reassemble`, drains any `Source` and hands back the finished turn:

```go
parsed := essessey.Reassemble(ctx, sse.NewSource(resp.Body))

fmt.Println(parsed.Text)      // every text delta, concatenated
fmt.Println(parsed.ToolNames) // tools the model called
```

The full walkthrough is in [Getting started](https://psyb0t.github.io/essessey/getting-started/).

## What's in the box

| Package | Does |
| --- | --- |
| `essessey` | `Event`, the `Sink`/`Source` interfaces, `Publisher`, the text, thinking and line streamers, `Reassemble`, `MultiSink`, `EventStore` and the in-memory sink, source and store |
| `essessey/sse` | the SSE format itself: framing, `WriterSink`, `HTTPSink` and a spec-following parser |
| `essessey/nats` | publishes payloads raw to `subjectPrefix.<event type>`, reads them back from a subscription |
| `essessey/ws` | one JSON frame per event, read back from your read loop |
| `essessey/elelemstream` | elelem's callbacks turned into correctly indexed blocks |

## Documentation

The full docs live at **[psyb0t.github.io/essessey](https://psyb0t.github.io/essessey/)**.

| Page | What's in it |
| --- | --- |
| [Getting started](https://psyb0t.github.io/essessey/getting-started/) | A first stream into an in-memory sink, swapping the delivery, reading it back |
| [Events and bindings](https://psyb0t.github.io/essessey/events-and-bindings/) | `Event`, `Sink`, `Source`, who frames and why, what each binding's receiver gets |
| [Event protocol](https://psyb0t.github.io/essessey/protocol/) | Every event, payload, block type and stop reason, and how block indexes work |
| [Publisher and streamers](https://psyb0t.github.io/essessey/publishing/) | The `Send*` methods, `TextStreamer`, `LineStreamer`, tool blocks, logging |
| [Reading a stream back](https://psyb0t.github.io/essessey/reassembly/) | `Reassemble`, `ParsedStream`, error and truncation behavior |
| [Resuming a dropped stream](https://psyb0t.github.io/essessey/resume/) | Event ids, `MultiSink`, `EventStore`, replay rules and the security boundary |
| [SSE binding](https://psyb0t.github.io/essessey/sse/) | Sinks, required headers, wire bytes, the parser, keep-alives |
| [NATS binding](https://psyb0t.github.io/essessey/nats/) | Subject scheme, subscribing, the missing id, ordering caveats |
| [WebSocket binding](https://psyb0t.github.io/essessey/websocket/) | Frames, receiving, the one-writer rule |
| [elelem adapter](https://psyb0t.github.io/essessey/elelemstream/) | `Adapter`, your own callbacks, stop reasons, the tool index arithmetic |
| [Development](https://psyb0t.github.io/essessey/development/) | Make targets, coverage gate, security scan, CI and releases |

## Development

Every target runs inside the `Dockerfile.dev` image, so a clean host needs Docker rather than a curated Go toolchain.

```bash
make lint          # go fix, then golangci-lint at full strictness
make test-coverage # the suite with -race plus the 90% coverage floor CI enforces
make sec           # govulncheck + semgrep, merged into sec.sarif
make help          # the rest of it
```

The rest is in [Development](https://psyb0t.github.io/essessey/development/). Release notes are in [CHANGELOG.md](https://github.com/psyb0t/essessey/blob/main/CHANGELOG.md).

## Sponsor

<a href="https://www.buymeacoffee.com/psyb0t"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-blue.png" alt="Buy Me a Coffee" height="60" width="217"></a>

## License

MIT. See [LICENSE](https://github.com/psyb0t/essessey/blob/main/LICENSE).
