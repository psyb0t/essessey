---
title: "Publisher and streamers"
description: "Emit an LLM turn with essessey's Publisher: the Send methods, TextStreamer and thinking streamers, LineStreamer transforms, tool blocks and logging."
nav_order: 5
---

# Publisher and streamers

The producing side. A `Publisher` turns protocol events into `Sink` emissions, and the streamers turn a chunk-at-a-time answer into correctly indexed content blocks so you never track an index by hand. The code is in [`publisher.go`](https://github.com/psyb0t/essessey/blob/main/publisher.go) and [`streamer.go`](https://github.com/psyb0t/essessey/blob/main/streamer.go).

## Publisher

```go
pub := essessey.NewPublisher(ctx, sink)
```

Build one per stream. The context you pass bounds the stream: it goes to every `Sink.Emit` call and is where the logger comes from, so the helpers don't thread a `ctx` through every call.

`Publish(eventType, data)` marshals `data` to JSON and emits it. Every `Send*` method goes through it. You only call it directly for an event type the `Send*` methods don't cover.

### Opening and closing a stream

| Method | Emits |
| --- | --- |
| `SendStreamPreamble(msgID, streamID, model)` | `message_start`, then `ping` |
| `SendStreamEpilogue(stopReason, outputTokens)` | `message_delta`, then `message_stop` |

Each half is also available alone: `SendMessageStart`, `SendPing`, `SendMessageDelta`, `SendMessageStop`.

### Blocks

| Method | Emits |
| --- | --- |
| `SendContentBlockStartText(index)` | a text block start |
| `SendContentBlockDeltaText(index, text)` | a `text_delta` |
| `SendContentBlockStartThinking(index)` | a thinking block start |
| `SendContentBlockDeltaThinking(index, text)` | a `thinking_delta` |
| `SendContentBlockStop(index)` | the stop for any block |
| `SendToolUseStart(index, toolUseID, name)` | a tool use block start with empty `input` |
| `SendToolInputDelta(index, inputJSON)` | an `input_json_delta` |
| `SendToolResultStart(index, toolUseID, isError)` | a tool result block start |
| `SendToolResultDelta(index, text)` | a `json_partial` delta |
| `SendToolUseBlock(index, toolUseID, name, inputJSON)` | start, one input delta and stop, in one call |
| `SendToolResultBlock(index, toolUseID, resultText, isError)` | start, one result delta and stop, in one call |

The payloads are on [Event protocol](protocol.md). Every error comes back wrapped with [ctxerrors](https://github.com/psyb0t/ctxerrors), so `errors.Is` still matches the sink's own error.

## TextStreamer

```go
text := essessey.NewTextStreamer(pub, startIndex)
thinking := essessey.NewThinkingStreamer(pub, startIndex)
```

A `TextStreamer` accumulates chunks and emits them as one block: text blocks from `NewTextStreamer`, reasoning blocks from `NewThinkingStreamer`. Both behave the same apart from the block and delta type.

- `Write(ctx, chunk)` ignores an empty chunk. On the first non-empty chunk it sends the block start, then a delta for every chunk.
- `Close(ctx)` sends the stop and advances the index by one. On a streamer that never opened a block it does nothing, which is what keeps the index honest for a round with no content of that kind.
- `BlockIndex()` is the next free index, `BlockStarted()` says whether a block is open, and `Text()` is everything written so far.

The publisher may be `nil`. The streamer then only accumulates, which is handy when the same code path serves a non-streaming caller that wants `Text()` at the end.

Chaining streamers is how a turn keeps its indexes straight. Close one, and start the next at its `BlockIndex()`:

```go
thinking := essessey.NewThinkingStreamer(pub, 0)
_ = thinking.Write(ctx, reasoning)

if err := thinking.Close(ctx); err != nil { // thinking closes before text opens
	return err
}

text := essessey.NewTextStreamer(pub, thinking.BlockIndex())
_ = text.Write(ctx, answer)

if err := text.Close(ctx); err != nil {
	return err
}

next := text.BlockIndex() // where a tool block goes
```

If `reasoning` was empty, no thinking block was emitted, `thinking.BlockIndex()` is still `0`, and the text block takes index `0`.

A streamer is not safe for concurrent use; feed each one from a single goroutine.

## LineStreamer

```go
lines := essessey.NewLineStreamer(pub, startIndex, transform)
```

The newline-buffered sibling, for line-oriented payloads where each line has to be checked or rewritten before the client sees it (a spec for a UI component, say). It always emits a text block.

- `Write(ctx, chunk)` splits at `\n`. Each complete line goes through the `LineTransform` (`func(line string) string`) and is emitted with its newline put back. A trailing partial line waits for the next chunk.
- `Close(ctx)` runs the trailing partial line through the transform, emits it without a newline, then sends the stop and advances the index.
- `Text()` returns the transformed output. A `nil` transform passes lines through unchanged.

The block still opens lazily: the first emitted line opens it, even if the transform returned an empty string for that line.

```go
upper := func(line string) string { return strings.ToUpper(line) }

lines := essessey.NewLineStreamer(pub, 0, upper)
_ = lines.Write(ctx, "first\nsec")
_ = lines.Write(ctx, "ond\nthird")
_ = lines.Close(ctx)

fmt.Printf("%q\n", lines.Text()) // "FIRST\nSECOND\nTHIRD"
```

## Tool calls

A tool call is two blocks: a tool use block with the arguments, and a tool result block with the output, matched by `toolUseID`. For one call after a text block:

```go
base := text.BlockIndex()

if err := pub.SendToolUseBlock(base, "call_1", "get_weather", `{"city":"Paris"}`); err != nil {
	return err
}

result, isError := runTool()

if err := pub.SendToolResultBlock(base+1, "call_1", result, isError); err != nil {
	return err
}

return pub.SendStreamEpilogue(essessey.StopReasonToolUse, outputTokens)
```

With several calls in one round, put every tool use block first and every tool result block after them. That arithmetic is the reason the [elelem adapter](elelemstream.md) exists; if you drive elelem, use it instead of writing this yourself.

## Logging

`Publish` writes one DEBUG line per event through the logger in the context ([ctxscope](https://github.com/psyb0t/ctxscope)). It carries the event name and, where the payload has them, the block index and block type. For deltas it logs `delta_len`, the length of the content, and never the content itself, so model output and tool payloads stay out of your logs. `message_delta` logs `stop_reason` and `output_tokens`.
