---
title: "Event protocol"
description: "Every essessey event name, its JSON payload, the content block and delta types, stop reasons, and how block indexes are assigned in a turn."
nav_order: 4
---

# Event protocol

What the `Publisher` puts in `Event.Event` and `Event.Data`. The constants and payload structs live in [`types.go`](https://github.com/psyb0t/essessey/blob/main/types.go). The event names are the same on every binding; only the delivery differs (see [Events and bindings](events-and-bindings.md)).

## A whole turn

One turn with reasoning, a short answer, one tool call and its result, as the [SSE binding](sse.md) writes it:

```text
event: message_start
data: {"type":"message_start","message":{"id":"msg_1","stream_id":"conv_1","type":"message","role":"assistant","content":[],"model":"some-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}

event: ping
data: {"type":"ping"}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","text":"Need the weather."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Checking."}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_1","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Paris\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":2}

event: content_block_start
data: {"type":"content_block_start","index":3,"content_block":{"type":"tool_result","tool_use_id":"call_1"}}

event: content_block_delta
data: {"type":"content_block_delta","index":3,"delta":{"type":"json_partial","text":"{\"temp_c\":18}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":3}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}

event: message_stop
data: {"type":"message_stop"}

```

Every payload repeats the event name in its own `type` field, so a receiver that only sees `Data` (a NATS subscriber, say) can still tell events apart.

## Events

| Event | Sent by | Payload |
| --- | --- | --- |
| `message_start` | `SendMessageStart`, `SendStreamPreamble` | `message`: `id`, `stream_id`, `type` (`message`), `role` (`assistant`), `content` (always `[]`), `model`, `stop_reason` and `stop_sequence` (always `null`), `usage` with `input_tokens` and `output_tokens` (always `0`) |
| `ping` | `SendPing`, `SendStreamPreamble` | only `type` |
| `content_block_start` | `SendContentBlockStartText`, `SendContentBlockStartThinking`, `SendToolUseStart`, `SendToolResultStart` | `index` and `content_block` (see below) |
| `content_block_delta` | `SendContentBlockDeltaText`, `SendContentBlockDeltaThinking`, `SendToolInputDelta`, `SendToolResultDelta` | `index` and `delta` (see below) |
| `content_block_stop` | `SendContentBlockStop` | `index` |
| `message_delta` | `SendMessageDelta`, `SendStreamEpilogue` | `delta` with `stop_reason` and `stop_sequence` (always `null`), `usage` with `output_tokens` |
| `message_stop` | `SendMessageStop`, `SendStreamEpilogue` | only `type` |

`stream_id` is your own identifier for whatever the stream represents: a conversation, a job, a document build. essessey never mints one and only echoes it back, so a client learns a new stream's id from the stream itself. It is also the natural key for an [`EventStore`](resume.md).

## Content blocks

| Block | `content_block` on start | Delta `type` | Delta field |
| --- | --- | --- | --- |
| Text | `{"type":"text"}` | `text_delta` | `text` |
| Thinking (reasoning) | `{"type":"thinking"}` | `thinking_delta` | `text` |
| Tool use | `{"type":"tool_use","id":"call_1","name":"get_weather","input":{}}` | `input_json_delta` | `partial_json` |
| Tool result | `{"type":"tool_result","tool_use_id":"call_1"}`, plus `"is_error":true` on a failed tool | `json_partial` | `text` |

The tool use start always carries an empty `input` object; the arguments arrive in the `input_json_delta`. A tool result is matched to its call by `tool_use_id`.

The block type constants (`ContentBlockTypeText`, `ContentBlockTypeToolUse` and the rest) are string aliases, not a closed enum. A producer may emit block types essessey has never heard of, and a consumer can name them without patching the package.

## Stop reasons

| Constant | Value | Meaning |
| --- | --- | --- |
| `StopReasonEndTurn` | `end_turn` | the model finished its answer |
| `StopReasonToolUse` | `tool_use` | the model asked for tools; the turn continues after their results |
| `StopReasonMaxTokens` | `max_tokens` | the answer was cut off by a token cap; do not treat it as complete |

`SendStreamEpilogue` takes the stop reason so the client learns why the stream ended instead of always being told `end_turn`. [`elelemstream.MapStopReason`](elelemstream.md#stop-reasons) derives it from an elelem finish reason.

## Block indexes

Every block in a turn has an index, and every delta and stop names the index it belongs to. The rules:

- Indexes count up from the index you start a streamer at, normally `0`.
- A block opens on its first non-empty chunk. A round that produces no text of a kind emits no block for it and uses no index. Get this wrong and the client renders a blank card, or every later block shifts by one.
- The index advances only when a block that opened is closed. `TextStreamer.BlockIndex()` after `Close` is the next free index.
- A thinking block closes before a text block opens. They are separate blocks; overlapping them makes a client render reasoning into the answer.
- Parallel tool calls in one round put all tool use blocks before all tool result blocks. [elelem adapter](elelemstream.md#the-arithmetic) has the layout.

[Publisher and streamers](publishing.md) shows how the streamers keep these rules for you.
