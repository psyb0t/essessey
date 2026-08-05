package essessey

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Lazy opening is the invariant the whole block protocol rests on: a round that
// produced no text of a given kind must emit NOTHING, or the client renders a
// blank card and the index advances for a block that never existed.
func TestTextStreamer_EmitsNothingWithoutContent(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewTextStreamer(NewPublisher(t.Context(), sink), 7)

	require.NoError(t, streamer.Write(t.Context(), ""))
	require.NoError(t, streamer.Close(t.Context()))

	assert.Zero(t, sink.Len(), "an empty streamer must emit no events")
	assert.Equal(t, 7, streamer.BlockIndex(),
		"a block that never opened must not advance the index")
	assert.False(t, streamer.BlockStarted())
}

func TestTextStreamer_OpensOnceThenDeltasAndStops(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewTextStreamer(NewPublisher(t.Context(), sink), 0)

	require.NoError(t, streamer.Write(t.Context(), "he"))
	require.NoError(t, streamer.Write(t.Context(), "llo"))
	require.NoError(t, streamer.Close(t.Context()))

	emitted := sink.Events()
	got := make([]EventType, 0, len(emitted))

	for _, ev := range emitted {
		got = append(got, ev.Event)
	}

	assert.Equal(t, []EventType{
		EventTypeContentBlockStart,
		EventTypeContentBlockDelta,
		EventTypeContentBlockDelta,
		EventTypeContentBlockStop,
	}, got, "the block opens once, not per chunk")

	assert.Equal(t, "hello", streamer.Text())
	assert.Equal(t, 1, streamer.BlockIndex())
}

// Closing twice must not emit a second stop or advance the index again — the
// adapter closes defensively, and a double-advance would shift every later
// block by one.
func TestTextStreamer_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewTextStreamer(NewPublisher(t.Context(), sink), 0)

	require.NoError(t, streamer.Write(t.Context(), "x"))
	require.NoError(t, streamer.Close(t.Context()))

	countAfterFirst := sink.Len()

	require.NoError(t, streamer.Close(t.Context()))

	assert.Equal(t, countAfterFirst, sink.Len())
	assert.Equal(t, 1, streamer.BlockIndex())
}

func TestThinkingStreamer_EmitsThinkingBlockTypes(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewThinkingStreamer(NewPublisher(t.Context(), sink), 0)

	require.NoError(t, streamer.Write(t.Context(), "pondering"))
	require.NoError(t, streamer.Close(t.Context()))

	events := sink.Events()
	require.Len(t, events, 3)

	assert.Contains(t, string(events[0].Data), ContentBlockTypeThinking)
	assert.Contains(t, string(events[1].Data), ContentBlockTypeThinkingDelta)
}

// A nil Publisher is the non-streaming path: the text still accumulates so a
// caller can persist the turn, but nothing goes on the wire.
func TestTextStreamer_NilPublisherAccumulatesSilently(t *testing.T) {
	t.Parallel()

	streamer := NewTextStreamer(nil, 0)

	require.NoError(t, streamer.Write(t.Context(), "kept"))
	require.NoError(t, streamer.Close(t.Context()))

	assert.Equal(t, "kept", streamer.Text())
}

func TestLineStreamer_EmitsCompleteLinesAndHoldsThePartial(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewLineStreamer(NewPublisher(t.Context(), sink), 0, nil)

	// A chunk split mid-line must not emit until the newline arrives.
	require.NoError(t, streamer.Write(t.Context(), "alpha\nbe"))

	assert.Equal(t, "alpha\n", streamer.Text(),
		"the partial line must be held back")

	require.NoError(t, streamer.Write(t.Context(), "ta\n"))
	require.NoError(t, streamer.Close(t.Context()))

	assert.Equal(t, "alpha\nbeta\n", streamer.Text())
}

// Close must flush a trailing line that never got its newline, or the last line
// of a stream silently disappears.
func TestLineStreamer_CloseFlushesUnterminatedTail(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewLineStreamer(NewPublisher(t.Context(), sink), 0, nil)

	require.NoError(t, streamer.Write(t.Context(), "no trailing newline"))
	require.NoError(t, streamer.Close(t.Context()))

	assert.Equal(t, "no trailing newline", streamer.Text())
	assert.Positive(t, sink.Len())
}

func TestLineStreamer_AppliesTheTransform(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	streamer := NewLineStreamer(
		NewPublisher(t.Context(), sink),
		0,
		strings.ToUpper,
	)

	require.NoError(t, streamer.Write(t.Context(), "shout\n"))
	require.NoError(t, streamer.Close(t.Context()))

	assert.Equal(t, "SHOUT\n", streamer.Text())
}
