---
title: "Reading a stream back"
description: "Reassemble any essessey Source into a ParsedStream: accumulated text and reasoning, tool calls matched to results, a timeline, and error behavior."
nav_order: 6
---

# Reading a stream back

The other half. `Reassemble` drains any `Source` and hands back the finished turn, so you do not walk events yourself unless you want to. The code is in [`reassemble.go`](https://github.com/psyb0t/essessey/blob/main/reassemble.go).

```go
src := sse.NewSource(resp.Body) // or a nats/ws Source, or a SliceSource in a test

parsed := essessey.Reassemble(ctx, src)

fmt.Println(parsed.Text)      // every text delta, concatenated
fmt.Println(parsed.Thinking)  // every reasoning delta, concatenated
fmt.Println(parsed.ToolNames) // tools the model called, in order
fmt.Println(parsed.Timeline)  // reasoning, text and tool activity, in order
```

## ParsedStream

| Field | Holds |
| --- | --- |
| `StreamID` | `stream_id` from `message_start` |
| `Thinking` | the content of every thinking block, concatenated |
| `Text` | the content of every text block, concatenated |
| `ToolNames` | the name of every tool use block, in the order they started |
| `Tools` | every tool use block as a `ToolCall`: `Name`, `Params` (the joined `input_json_delta` fragments as `json.RawMessage`, `{}` when there were none) and `ToolUseID` |
| `Executions` | every tool call matched to its result as a `ToolExecution`: `Name`, `Params`, `Result`, `ToolUseID` |
| `Timeline` | `TimelineItem` entries in stream order: `Kind` is `thinking`, `text` or `tool`; `Text` holds the segment for the first two, `Execution` the matched call for `tool` |
| `Error` | the error text when the source failed, empty otherwise |

Separate thinking blocks stay separate timeline entries, while `Thinking` holds their concatenated content. Same for text.

A tool result is matched to its call by `tool_use_id`. A result whose call never appeared (a truncated upstream, or two streams rejoined) is logged and dropped rather than turned into an execution with no name or parameters.

## When it stops

- `message_stop` ends reassembly right away, even if the source has more.
- `ErrNoMoreEvents` from the source ends it cleanly. Blocks that never got a `content_block_stop` are closed as if they had, so a truncated stream still returns what arrived.
- Any other error from `Next` stops reassembly and is recorded in `ParsedStream.Error`. `Reassemble` itself never returns an error; check `Error`.

A malformed event (JSON that does not parse, a thinking, tool input or tool result delta for a block that is not open, a stop with no open block, an unknown block or delta type) is warn-logged with a `reason` field and dropped. One corrupted event degrades the result instead of failing the caller.

`Reassemble` blocks for as long as the source does. The NATS and WebSocket sources wait in `Next` until an event arrives, the source is closed, or the context is done, so pass a context with a deadline or close the source when the producer is gone.

## Walking events yourself

A `Source` is just an iterator:

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

Unmarshal `ev.Data` into the payload structs from [`types.go`](https://github.com/psyb0t/essessey/blob/main/types.go) (`MessageStartData`, `ContentBlockDeltaData` and the rest) to read a payload. [Event protocol](protocol.md) lists which struct fits which event.
