// Package sse is the SSE FORMAT binding for essessey: the wire codec plus the
// byte-stream Sinks (WriterSink, HTTPSink) and the byte-stream Source that
// frame/parse it.
//
// SSE is a FORMAT, not a transport peer of NATS/WebSocket. Its framing exists
// because an HTTP response body is an undelimited byte stream and something
// must mark where one event ends — message-oriented transports need none. So
// this package owns ALL framing; other bindings own none.
package sse

import (
	"fmt"

	"github.com/psyb0t/essessey"
)

// SSE wire framing. The blank line terminates an event; the prefixes are the
// protocol contract shared by every Sink and the Source in this package.
const (
	framePrefixEvent = "event: "
	framePrefixData  = "data: "
)

// FrameLines renders the canonical wire bytes for ev: an `event:` line, a
// `data:` line, then a blank line terminator. Every Sink and the Source in
// this package agree on this exact format so they can never drift — a live
// frontend consumes exactly these bytes.
func FrameLines(ev essessey.Event) string {
	return fmt.Sprintf(
		"%s%s\n%s%s\n\n",
		framePrefixEvent, ev.Event,
		framePrefixData, string(ev.Data),
	)
}
