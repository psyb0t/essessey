package essessey

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSinkRefused = errors.New("sink refused")

// flakySink fails on the Nth Emit (1-based) and succeeds otherwise, so a
// composite helper can be stopped at each of its steps in turn.
type flakySink struct {
	failOn int
	calls  int
}

func (s *flakySink) Emit(context.Context, Event) error {
	s.calls++

	if s.calls == s.failOn {
		return errSinkRefused
	}

	return nil
}

// A composite helper emits several events. If one fails partway, it MUST stop
// and surface the error rather than pressing on — a half-written block leaves
// the client rendering a card that never closes.
func TestPublisher_CompositeHelpersStopAtTheFailingStep(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		send      func(*Publisher) error
		failOn    int
		wantCalls int
	}{
		{
			"preamble fails on message_start",
			func(p *Publisher) error {
				return p.SendStreamPreamble("m", "c", "model")
			},
			1, 1,
		},
		{
			"epilogue fails on message_delta",
			func(p *Publisher) error {
				return p.SendStreamEpilogue(StopReasonEndTurn, 1)
			},
			1, 1,
		},
		{
			"tool_use block fails on start",
			func(p *Publisher) error {
				return p.SendToolUseBlock(0, "c1", "lookup", "{}")
			},
			1, 1,
		},
		{
			"tool_use block fails on the input delta",
			func(p *Publisher) error {
				return p.SendToolUseBlock(0, "c1", "lookup", "{}")
			},
			2, 2,
		},
		{
			"tool_result block fails on start",
			func(p *Publisher) error {
				return p.SendToolResultBlock(0, "c1", "out", false)
			},
			1, 1,
		},
		{
			"tool_result block fails on the delta",
			func(p *Publisher) error {
				return p.SendToolResultBlock(0, "c1", "out", false)
			},
			2, 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := &flakySink{failOn: tc.failOn}

			err := tc.send(NewPublisher(t.Context(), sink))

			require.ErrorIs(t, err, errSinkRefused)
			assert.Equal(t, tc.wantCalls, sink.calls,
				"must stop at the failing step, not keep emitting")
		})
	}
}

// The LineStreamer exposes the same block accounting as TextStreamer, and the
// adapter reads it to decide where the next block starts.
func TestLineStreamer_ReportsBlockState(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewLineStreamer(NewPublisher(t.Context(), sink), 5, nil)

	assert.Equal(t, 5, streamer.BlockIndex())
	assert.False(t, streamer.BlockStarted())

	require.NoError(t, streamer.Write(t.Context(), "line\n"))

	assert.True(t, streamer.BlockStarted(),
		"a written line must open the block")

	require.NoError(t, streamer.Close(t.Context()))

	assert.False(t, streamer.BlockStarted())
	assert.Equal(t, 6, streamer.BlockIndex(),
		"closing an opened block advances the index exactly once")
}

// An empty write is a no-op — it must not open a block, or a round with no
// line-oriented content would still burn an index.
func TestLineStreamer_EmptyWriteOpensNothing(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewLineStreamer(NewPublisher(t.Context(), sink), 0, nil)

	require.NoError(t, streamer.Write(t.Context(), ""))
	require.NoError(t, streamer.Close(t.Context()))

	assert.Zero(t, sink.Len())
	assert.Equal(t, 0, streamer.BlockIndex())
}
