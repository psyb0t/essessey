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

// rawMalformed carries: an orphan data: line (no preceding event: line), an
// event: line immediately superseded by another event: line before any
// data: line (the first is incomplete and gets dropped), then one valid
// frame.
const rawMalformed = `data: orphan

event: ping
event: message_stop
data: {"type":"message_stop"}

`

func TestSource_Next_SkipsMalformedFrames(t *testing.T) {
	t.Parallel()

	source := NewSource(strings.NewReader(rawMalformed))

	ev, err := source.Next(t.Context())
	require.NoError(t, err)
	assert.Equal(t, essessey.EventTypeMessageStop, ev.Event)
	assert.JSONEq(t, `{"type":"message_stop"}`, string(ev.Data))

	_, err = source.Next(t.Context())
	require.ErrorIs(t, err, essessey.ErrNoMoreEvents)
}
