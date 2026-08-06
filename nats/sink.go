package nats

import (
	"context"

	"github.com/psyb0t/common-go/scope"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/essessey"
)

// subjectSeparator joins a subject prefix and an event type into one
// NATS subject.
const subjectSeparator = "."

// Sink publishes essessey events to NATS subjects.
//
// NATS already delimits messages, so Emit publishes ev.Data RAW — no
// SSE-style framing. The subject is the configured prefix plus the
// event's type, so a subscriber can filter with a NATS wildcard subject
// (subjectPrefix.*) without inspecting payloads.
//
// A real *nats.Conn is safe for concurrent Publish calls, so Emit needs
// no mutex of its own — it holds no other mutable state.
type Sink struct {
	publisher     Publisher
	subjectPrefix string
}

// NewSink returns a Sink that publishes through p, with subjects prefixed
// by subjectPrefix. An empty subjectPrefix publishes to the bare event
// type.
func NewSink(p Publisher, subjectPrefix string) *Sink {
	return &Sink{publisher: p, subjectPrefix: subjectPrefix}
}

// Emit publishes ev.Data, unframed, to the subject derived from the
// configured prefix and ev.Event.
func (s *Sink) Emit(ctx context.Context, ev essessey.Event) error {
	logger := scope.GetLogger(ctx)

	subject := s.subject(ev.Event)

	if err := s.publisher.Publish(subject, ev.Data); err != nil {
		return ctxerrors.Wrap(err, "publish event")
	}

	logger.Debug(
		"published event",
		"subject", subject,
		"event", ev.Event,
	)

	return nil
}

func (s *Sink) subject(eventType essessey.EventType) string {
	if s.subjectPrefix == "" {
		return eventType
	}

	return s.subjectPrefix + subjectSeparator + eventType
}
