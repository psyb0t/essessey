# Errors

Prefix: ERR. Sentinels sit next to the code that returns them: `ErrNoMoreEvents` in `event.go`, `ErrInvalidCapacity` in `store.go`, `ErrNotAFlusher` in `sse/errors.go`, `ErrRoundStreamNotInitialized` and `ErrToolResultMissing` in `elelemstream/errors.go`.

- **ERR1:** Wrap every returned error with `ctxerrors.Wrap` or `ctxerrors.Wrapf` and a short action phrase (`"emit event"`, `"send stream preamble"`). `wrapcheck` and `err113` are on, so a bare third-party error or an inline `errors.New` at a call site fails lint.
- **ERR2:** A new failure a caller may want to test for gets an exported `Err*` sentinel with a doc comment saying what it means and why it is refused, then is returned wrapped so `errors.Is` still matches.
- **ERR3:** `ErrNoMoreEvents` is the clean end of a `Source`, not a failure. Every `Source` returns it when its input ends normally, and `Reassemble` treats it as the end of the turn. Any other error from `Next` is recorded on `ParsedStream.Error` and stops reassembly.
- **ERR4:** Reject bad input at construction rather than later (`NewInMemoryEventStore` refuses a capacity of zero or less, `sse.NewHTTPSink` refuses a writer that is not an `http.Flusher`).
- **ERR5:** A malformed event on the read side is warn-logged with a `reason` field and dropped; it does not abort the stream (`Reassemble`, `sse.Source`). Keep that behavior for new event shapes.
