package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/psyb0t/common-go/scope"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/essessey"
)

// maxLineSize bounds a single scanned SSE line (1 MiB) so a pathological
// upstream can't drive unbounded allocation.
const maxLineSize = 1024 * 1024

// Source reads framed SSE bytes off an io.Reader and yields essessey.Event —
// the read-side mirror of WriterSink/HTTPSink. A malformed frame (an orphan
// data: line, or an event: line never followed by a data: line before the
// next event: line or EOF) is WARN-logged and SKIPPED rather than aborting
// the read: one corrupted frame should degrade the stream, not the caller.
type Source struct {
	scanner *bufio.Scanner

	pendingEvent essessey.EventType
	hasPending   bool
}

// NewSource builds a Source scanning framed SSE bytes out of r.
func NewSource(r io.Reader) *Source {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

	return &Source{scanner: scanner}
}

// Next returns the next framed event, or essessey.ErrNoMoreEvents once the
// underlying reader is exhausted cleanly.
func (s *Source) Next(ctx context.Context) (essessey.Event, error) {
	for s.scanner.Scan() {
		line := s.scanner.Text()

		if name, ok := strings.CutPrefix(line, framePrefixEvent); ok {
			s.startEvent(ctx, name)

			continue
		}

		data, ok := strings.CutPrefix(line, framePrefixData)
		if !ok {
			continue
		}

		if ev, ok := s.completeEvent(ctx, data); ok {
			return ev, nil
		}
	}

	if err := s.scanner.Err(); err != nil {
		return essessey.Event{}, ctxerrors.Wrap(err, "scan sse stream")
	}

	s.dropPending(ctx)

	return essessey.Event{}, essessey.ErrNoMoreEvents
}

// startEvent records a new pending event name, warning + dropping any
// previous pending event that never received a data: line.
func (s *Source) startEvent(ctx context.Context, name essessey.EventType) {
	s.dropPending(ctx)

	s.pendingEvent = name
	s.hasPending = true
}

// completeEvent pairs a data: line with the pending event. Reports ok=false
// (and warns) for an orphan data: line with no preceding event: line.
func (s *Source) completeEvent(
	ctx context.Context,
	data string,
) (essessey.Event, bool) {
	if !s.hasPending {
		scope.GetLogger(ctx).Warn(
			"sse source: dropping orphan data line",
			"reason", "orphan_data",
		)

		return essessey.Event{}, false
	}

	ev := essessey.Event{
		Event: s.pendingEvent,
		Data:  json.RawMessage(data),
	}
	s.hasPending = false

	return ev, true
}

// dropPending warns and clears a pending event that never received a data:
// line, if one is currently pending.
func (s *Source) dropPending(ctx context.Context) {
	if !s.hasPending {
		return
	}

	scope.GetLogger(ctx).Warn(
		"sse source: dropping incomplete event, no data line",
		"event", s.pendingEvent,
		"reason", "incomplete_event",
	)

	s.hasPending = false
}
