package ws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSource_DeliversInOrderThenNoMoreEvents(t *testing.T) {
	t.Parallel()

	src := NewSource()

	events := []essessey.Event{
		{Event: "message_start", Data: json.RawMessage(`1`)},
		{Event: "content_block_delta", Data: json.RawMessage(`2`)},
	}

	for _, ev := range events {
		src.Deliver(ev)
	}

	src.Close()

	for _, want := range events {
		got, err := src.Next(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	_, err := src.Next(context.Background())
	require.ErrorIs(t, err, essessey.ErrNoMoreEvents)
}

func TestSource_Close_IsIdempotent(t *testing.T) {
	t.Parallel()

	src := NewSource()

	src.Close()
	assert.NotPanics(t, func() { src.Close() })

	_, err := src.Next(context.Background())
	require.ErrorIs(t, err, essessey.ErrNoMoreEvents)
}

func TestSource_Deliver_AfterCloseIsNoop(t *testing.T) {
	t.Parallel()

	src := NewSource()

	src.Close()
	assert.NotPanics(t, func() {
		src.Deliver(essessey.Event{Event: testEventTypePing})
	})

	_, err := src.Next(context.Background())
	require.ErrorIs(t, err, essessey.ErrNoMoreEvents)
}

func TestSource_Next_ContextCancelled(t *testing.T) {
	t.Parallel()

	src := NewSource()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Next(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
