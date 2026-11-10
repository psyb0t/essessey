# sse

The SSE format binding for essessey: `FrameLines`, `FrameComment` and `FrameRetry` render `text/event-stream` as the WHATWG HTML Living Standard defines it, `WriterSink` and `HTTPSink` write framed events to an `io.Writer` or a flushing `http.ResponseWriter`, and `Source` parses them back off an `io.Reader`.

Your handler must set `Content-Type: text/event-stream` itself; this package does not.

Full documentation, including the exact wire bytes, the parser rules and the things that will bite you: **[SSE binding](https://psyb0t.github.io/essessey/sse/)**.
