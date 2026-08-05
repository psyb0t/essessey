package sse

import (
	"encoding/json"
	"testing"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
)

func TestFrameLines(t *testing.T) {
	t.Parallel()

	ev := essessey.Event{
		Event: essessey.EventTypePing,
		Data:  json.RawMessage(`{"type":"ping"}`),
	}

	got := FrameLines(ev)

	assert.Equal(t, "event: ping\ndata: {\"type\":\"ping\"}\n\n", got)
}
