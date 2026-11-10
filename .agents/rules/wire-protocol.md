# Wire protocol

Prefix: WIRE. The protocol lives in `event.go` and `types.go`; the SSE format lives in `sse/`.

- **WIRE1:** `Event` is the only thing a binding carries: `ID` (`json:"id,omitempty"`), `Event` and `Data` (`json.RawMessage`). Do not add delivery details to it. An empty id must never be written, because in SSE an empty `id:` resets the client's resume point.
- **WIRE2:** Only `sse/` frames. `nats.Sink` publishes `Event.Data` raw to `<prefix>.<event type>` (the id is not carried), and `ws.Sink` writes the whole `Event` as one JSON object. Do not add framing to a message-oriented binding.
- **WIRE3:** SSE output goes through `FrameLines`, `FrameComment` and `FrameRetry` only. They split a multi-line payload into one `data:` line per line, and strip CR, LF and NUL from `id`, `event` and comment text so a value can never inject extra fields or events. Never write `data:` lines by hand.
- **WIRE4:** `sse.Source` follows the WHATWG parsing algorithm: an event ends at a blank line, the space after the colon is optional, CRLF, LF and lone CR all end a line, a leading BOM is stripped, an event with no data is discarded, and the last event id persists across events. A parser change needs a case in `sse/conformance_test.go`.
- **WIRE5:** Blocks open lazily on the first non-empty chunk, and the block index advances only when a block that opened is closed. A thinking block closes before a text block opens. `TextStreamer`, `LineStreamer` and `elelemstream` all depend on this; keep it when changing any of them.
- **WIRE6:** In one `elelemstream` round, tool blocks are laid out from `toolBase`: `toolBase + i` for tool_use of call `i`, `toolBase + toolCallCount + i` for its tool_result, and the next round starts at `toolBase + 2*toolCallCount`, advanced once on the last result. `elelemstream/adapter_test.go` asserts these indices.
- **WIRE7:** Adding an event or block type means: the constant and payload struct in `types.go`, a `Send*` method on `Publisher`, a `logFields` case, handling in `Reassemble`, tests, and the protocol page in `docs/`.
- **GO7:** Protocol string types (`EventType`, `ContentBlockType`, `StopReason`, `Role`, `TimelineItemKind`) are aliases of `string`, not defined types, so a producer can emit a value this package does not know. Keep new ones as aliases.
- **GO8:** JSON tags on wire types are snake_case to match the protocol. `types.go` carries a file-level `//nolint:tagliatelle` for that; a new wire struct elsewhere needs the same.
