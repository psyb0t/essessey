package ws

import (
	"context"
	"sync"

	"github.com/psyb0t/common-go/scope"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/essessey"
)

// Sink writes essessey events to a WebSocket connection.
//
// WebSocket already delimits frames, so Emit sends the whole
// essessey.Event as one WriteJSON call — no SSE-style framing. The
// client receives {"event": ..., "data": ...} on the wire.
//
// Unlike a NATS connection, gorilla's *websocket.Conn permits only one
// writer at a time, so Emit is mutex-guarded to stay safe for concurrent
// use.
type Sink struct {
	mu   sync.Mutex
	conn Conn
}

// NewSink returns a Sink that writes through c.
func NewSink(c Conn) *Sink {
	return &Sink{conn: c}
}

// Emit writes ev to the connection as a single JSON message.
func (s *Sink) Emit(ctx context.Context, ev essessey.Event) error {
	logger := scope.GetLogger(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.conn.WriteJSON(ev); err != nil {
		return ctxerrors.Wrap(err, "write event")
	}

	logger.Debug("wrote event", "event", ev.Event)

	return nil
}
