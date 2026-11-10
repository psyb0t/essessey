# Errors

Prefix: ERR. Sentinels sit next to the code that returns them: `ErrNoMoreEvents` in `event.go`, `ErrInvalidCapacity` in `store.go`, `ErrNotAFlusher` in `sse/errors.go`, `ErrRoundStreamNotInitialized` and `ErrToolResultMissing` in `elelemstream/errors.go`.

- **ERR3:** `ErrNoMoreEvents` is the clean end of a `Source`, not a failure. Every `Source` returns it when its input ends normally, and `Reassemble` treats it as the end of the turn. Any other error from `Next` is recorded on `ParsedStream.Error` and stops reassembly.
- **ERR4:** Reject bad input at construction rather than later (`NewInMemoryEventStore` refuses a capacity of zero or less, `sse.NewHTTPSink` refuses a writer that is not an `http.Flusher`).
- **ERR5:** A malformed event on the read side is warn-logged with a `reason` field and dropped; it does not abort the stream (`Reassemble`, `sse.Source`). Keep that behavior for new event shapes.
