package essessey_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errSinkOne = errors.New("sink one failed")
	errSinkTwo = errors.New("sink two failed")
)

// failingSink records what it was asked to emit and then refuses, so a test can
// prove a broken destination still SAW the event and that the failure surfaced.
type failingSink struct {
	err      error
	received []essessey.Event
}

func (s *failingSink) Emit(_ context.Context, ev essessey.Event) error {
	s.received = append(s.received, ev)

	return s.err
}

func testEvent(id string) essessey.Event {
	return essessey.Event{
		ID:    id,
		Event: "chunk",
		Data:  json.RawMessage(`{}`),
	}
}

func TestMultiSink_FansOutToEverySink(t *testing.T) {
	t.Parallel()

	first := essessey.NewInMemorySink()
	second := essessey.NewInMemorySink()
	ctx := context.Background()

	multi := essessey.NewMultiSink(first, second)

	require.NoError(t, multi.Emit(ctx, testEvent("1")))
	require.NoError(t, multi.Emit(ctx, testEvent("2")))

	require.Len(t, first.Events(), 2)
	require.Len(t, second.Events(), 2)
	assert.Equal(t, "1", first.Events()[0].ID)
	assert.Equal(t, "2", second.Events()[1].ID)
}

func TestMultiSink_ZeroSinksDiscards(t *testing.T) {
	t.Parallel()

	// Allowed so a caller can build the list conditionally without
	// special-casing empty.
	multi := essessey.NewMultiSink()

	require.NoError(t, multi.Emit(context.Background(), testEvent("1")))
}

func TestMultiSink_OneFailureDoesNotStopTheRest(t *testing.T) {
	t.Parallel()

	// The policy that matters: a broken store or audit sink must not cost the
	// user their stream. Every later sink still receives the event.
	broken := &failingSink{err: errSinkOne}
	wire := essessey.NewInMemorySink()
	ctx := context.Background()

	multi := essessey.NewMultiSink(broken, wire)

	err := multi.Emit(ctx, testEvent("1"))

	require.Error(t, err, "the failure must not be swallowed")
	require.ErrorIs(t, err, errSinkOne)
	require.Len(t, wire.Events(), 1,
		"a sink after the failing one must still receive the event")
	assert.Equal(t, "1", wire.Events()[0].ID)
}

func TestMultiSink_JoinsEveryFailure(t *testing.T) {
	t.Parallel()

	// Two broken sinks must both be reportable — returning only the first would
	// hide the second destination being down.
	first := &failingSink{err: errSinkOne}
	second := &failingSink{err: errSinkTwo}
	wire := essessey.NewInMemorySink()

	multi := essessey.NewMultiSink(first, second, wire)

	err := multi.Emit(context.Background(), testEvent("1"))

	require.Error(t, err)
	assert.ErrorIs(t, err, errSinkOne)
	assert.ErrorIs(t, err, errSinkTwo)
	assert.Len(t, wire.Events(), 1)

	// Both failing sinks were still ASKED — a failure is not a skip.
	assert.Len(t, first.received, 1)
	assert.Len(t, second.received, 1)
}

func TestMultiSink_CapturesToAStoreAlongsideTheWire(t *testing.T) {
	t.Parallel()

	// The motivating composition: one Emit reaches the client and the retention
	// buffer, with no wrapper type in between.
	store, err := essessey.NewInMemoryEventStore(10)
	require.NoError(t, err)

	wire := essessey.NewInMemorySink()
	ctx := context.Background()

	live := essessey.NewMultiSink(wire, store.SinkFor(testStreamID))

	for _, id := range []string{"1", "2", "3"} {
		require.NoError(t, live.Emit(ctx, testEvent(id)))
	}

	assert.Len(t, wire.Events(), 3)

	replayed, known, err := store.Since(ctx, testStreamID, "1")
	require.NoError(t, err)
	require.True(t, known)
	assert.Equal(t, []string{"2", "3"}, idsOf(replayed))
}
