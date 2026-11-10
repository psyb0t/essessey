# Logging

Prefix: LOG. This library never builds its own logger.

- **LOG1:** Never log model output, reasoning or tool payloads. `Publisher.Publish` logs one DEBUG line per event with the event name, block index, block type and, for deltas, only `delta_len`. A new `Send*` payload shape gets a case in `logFields` that follows the same rule.
- **LOG2:** Dropped or ignored input is logged at WARN with a stable `reason` field (`malformed_event`, `orphan_delta`, `orphan_stop`, `orphan_result`, `unknown_block_type`, `unknown_delta_shape`, `invalid_id`, `invalid_retry`, `retry_overflow`, `incomplete_event`, `source_closed`). Reuse an existing reason when it fits; add a new one only for a new kind of drop.
- **LOG3:** Code that has no request context, such as `Deliver` on the NATS and WebSocket sources, logs through `ctxscope.GetLogger(context.Background())`.
