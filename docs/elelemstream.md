---
title: "elelem adapter"
description: "The essessey elelemstream package: bind elelem's streaming callbacks to indexed content blocks, add your own hooks, map stop reasons, and the tool math."
nav_order: 8
---

# elelem adapter

[elelem](https://github.com/psyb0t/elelem) streams a turn as callbacks: deltas arrive, then tool calls, then their results. It has no notion of a content block index, and no reason to; that's a wire concern.

`github.com/psyb0t/essessey/elelemstream` is the seam. `Adapter` registers the callbacks, tracks where the turn is, and emits correctly indexed blocks through an `essessey.Publisher`. It is the one package in this module that imports elelem; the core package and the transport bindings stay free of it, so a caller who isn't using elelem never pulls it in. The code is in [`elelemstream/`](https://github.com/psyb0t/essessey/tree/main/elelemstream).

```go
pub := essessey.NewPublisher(ctx, sink)
adapter := elelemstream.New(pub)

if err := pub.SendStreamPreamble(msgID, streamID, model); err != nil {
	return err
}

resp, err := adapter.Bind(
	elelem.NewRequest(client).WithModel(model).WithPrompt(prompt),
).Run(ctx)
```

The adapter emits content blocks only. Opening and closing the stream (`SendStreamPreamble`, `SendStreamEpilogue`) stays with you, because only you know the message id, the stream id and when the turn is really over.

Build one `Adapter` per in-flight request. Its fields track the current round and are not safe to share. `Rounds()` reports how many rounds have completed.

## Adding your own callbacks

An app usually has per-round concerns of its own: a heartbeat, a log line, a metric. Register them as well. elelem's `On*` setters append to a chain, so both the adapter's callback and yours run, in registration order:

```go
adapter.Bind(req).
	OnRoundStart(func(context.Context, *elelem.RoundEvent) error {
		heartbeat.Touch()

		return nil
	})
```

This needs elelem **v0.3.0 or later** (the module currently requires v0.4.1). Before v0.3.0 the setters replaced rather than appended, and registering your own hook silently unregistered the adapter's: the stream just stopped emitting blocks, with no error to catch.

Every callback is also exported (`OnRoundStart`, `OnDelta`, `OnAssistantMessage`, `OnRoundEnd`, `OnToolCallStart`, `OnToolResult`), for when you want your hook at an exact point relative to the adapter's, or want to wire only some of them:

```go
req.OnRoundStart(func(ctx context.Context, ev *elelem.RoundEvent) error {
	heartbeat.Touch()

	return adapter.OnRoundStart(ctx, ev) // yours first, then the adapter's
})
```

## When elelem is not streaming

`elelem.WithStreaming(false)` exists for backends that cannot serve a streaming call, typically an async job queue in front of the model. It changes the transport, not the callbacks: elelem feeds the finished response through the same `On*` hooks the adapter is bound to.

So the adapter keeps working, and the block protocol on the wire has the same shape. The only difference a subscriber sees is timing: the blocks for a turn all arrive at once, at the end, instead of filling in as the model writes.

## Stop reasons

```go
reason := elelemstream.MapStopReason(resp.FinishReason, len(resp.ToolCalls) > 0)

return pub.SendStreamEpilogue(reason, outputTokens)
```

`MapStopReason(finishReason, hasToolCalls)` maps an elelem finish reason onto a [stop reason](protocol.md#stop-reasons): a truncated finish (`IsTruncated()`, meaning length or context exceeded) gives `max_tokens`, otherwise tool calls give `tool_use`, otherwise `end_turn`. Truncation wins over tool calls: a client must not treat a cut-off answer as a clean tool round just because calls happened to be in it.

## The arithmetic

This is a package and not ten lines in your app because the index arithmetic is fiddly, and silently wrong when you get it slightly off. A tool result landing on the wrong index does not error. It renders into the wrong card, and you find out from a screenshot.

A round emits its thinking and text blocks first. Wherever those stop becomes `toolBase`, and every tool block in the round is placed relative to it:

```text
toolBase + i                    tool_use for call i
toolBase + toolCallCount + i    tool_result for call i
toolBase + 2*toolCallCount      where the NEXT round starts
```

All tool use blocks come before any tool result block, which is why the result offset carries `+ toolCallCount` rather than putting each result right after its call. With two parallel calls the round lays out as:

| Index | Block |
| --- | --- |
| `toolBase + 0` | tool_use, call 0 |
| `toolBase + 1` | tool_use, call 1 |
| `toolBase + 2` | tool_result, call 0 |
| `toolBase + 3` | tool_result, call 1 |

Naive implementations interleave use and result pairs and look correct for a single tool call. Parallel calls are where they break.

## Invariants

- **Blocks open lazily.** A round with no reasoning emits no empty thinking block and uses no index for one.
- **Thinking closes before text opens.** They are separate blocks; overlapping them makes the client render reasoning into the answer. Reasoning that arrives after the round's text has started is not emitted.
- **The index advances only on a block that actually opened.** `Close` on an unopened streamer is a no-op; otherwise a silent round would shift everything after it.
- **The round advances once, on the last tool result** (`event.Index == toolCallCount-1`), not per result.

[`adapter_test.go`](https://github.com/psyb0t/essessey/blob/main/elelemstream/adapter_test.go) asserts the emitted indexes for the single-call, parallel-call, non-zero `toolBase` and second-round cases. Breaking either the result offset or the round advance fails them.

## What it does not do

Heartbeats, persistence, chat identifiers, logging of application state: those belong to the app that owns the turn, not to the translation layer. This package converts callbacks into blocks and nothing else.

## Errors

| Error | Means |
| --- | --- |
| `ErrRoundStreamNotInitialized` | a delta or assistant message arrived before `OnRoundStart` opened a round; the request was built without `Bind`, or without the adapter's `OnRoundStart` |
| `ErrToolResultMissing` | `OnToolResult` fired with no result attached; the adapter will not invent an empty one |

Both come back wrapped with call-site context, so `errors.Is` works through the wrap. An error from the publisher is wrapped and returned from the callback, which hands it back to elelem.
