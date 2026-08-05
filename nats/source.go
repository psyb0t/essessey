package nats

import (
	"context"
	"sync"

	"github.com/psyb0t/common-go/scope"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/essessey"
)

// sourceBufferSize sets how many delivered events queue before Deliver
// blocks its caller (typically a NATS subscription callback).
const sourceBufferSize = 64

// Source turns a NATS subscription's push-callback delivery into an
// essessey.Source. Wire Deliver as the subscription callback; Next pulls
// events off in the order they arrived.
type Source struct {
	mu     sync.Mutex
	ch     chan essessey.Event
	closed bool
}

// NewSource returns a ready-to-use Source.
func NewSource() *Source {
	return &Source{ch: make(chan essessey.Event, sourceBufferSize)}
}

// Deliver pushes ev onto the queue for Next to pick up. Deliver after
// Close is a no-op — the source has already stopped accepting events.
func (s *Source) Deliver(ev essessey.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		scope.GetLogger(context.Background()).Warn(
			"dropping delivered event, source is closed",
			"event", ev.Event,
			"reason", "source_closed",
		)

		return
	}

	s.ch <- ev
}

// Close stops the source. Next drains any already-buffered events first,
// then returns essessey.ErrNoMoreEvents. Close is idempotent.
func (s *Source) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true

	close(s.ch)
}

// Next blocks until an event is delivered, the source is closed and
// drained, or ctx is done.
func (s *Source) Next(ctx context.Context) (essessey.Event, error) {
	select {
	case ev, ok := <-s.ch:
		if !ok {
			return essessey.Event{}, essessey.ErrNoMoreEvents
		}

		return ev, nil
	case <-ctx.Done():
		return essessey.Event{}, ctxerrors.Wrap(
			ctx.Err(), "next: context done",
		)
	}
}
