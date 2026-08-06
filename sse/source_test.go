package sse

import (
	"strings"
	"testing"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSource_Next_EmptyInputReturnsNoMoreEvents(t *testing.T) {
	t.Parallel()

	source := NewSource(strings.NewReader(""))

	_, err := source.Next(t.Context())

	require.ErrorIs(t, err, essessey.ErrNoMoreEvents)
}

// rawUnusualButValid carries two things an earlier implementation treated as
// malformed and skipped, and that the format actually defines:
//
//   - A `data:` line with no preceding `event:` line. The event type is simply
//     absent; a receiver substitutes its own default. It is a real event and
//     must be delivered, not dropped.
//   - Two `event:` lines before the data. Each one SETS the event-type buffer,
//     so the last one wins. Neither is an error and nothing is discarded.
const rawUnusualButValid = `data: orphan

event: ping
event: message_stop
data: {"type":"message_stop"}

`

func TestSource_Next_UnusualButValidFrames(t *testing.T) {
	t.Parallel()

	source := NewSource(strings.NewReader(rawUnusualButValid))

	// The typeless event is delivered rather than skipped.
	ev, err := source.Next(t.Context())
	require.NoError(t, err)
	assert.Empty(t, ev.Event)
	assert.Equal(t, "orphan", string(ev.Data))

	// Repeated event fields: last one wins.
	ev, err = source.Next(t.Context())
	require.NoError(t, err)
	assert.Equal(t, essessey.EventTypeMessageStop, ev.Event)
	assert.JSONEq(t, `{"type":"message_stop"}`, string(ev.Data))

	_, err = source.Next(t.Context())
	require.ErrorIs(t, err, essessey.ErrNoMoreEvents)
}
