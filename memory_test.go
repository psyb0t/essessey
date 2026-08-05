package essessey

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemorySink_CollectsInOrder(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()

	require.NoError(t, sink.Emit(t.Context(), Event{Event: "a"}))
	require.NoError(t, sink.Emit(t.Context(), Event{Event: "b"}))

	events := sink.Events()
	require.Len(t, events, 2)
	assert.Equal(t, EventType("a"), events[0].Event)
	assert.Equal(t, EventType("b"), events[1].Event)
	assert.Equal(t, 2, sink.Len())
}

// Events hands out a COPY. Without that, a caller ranging over the result races
// a concurrent Emit, and editing an entry would rewrite the sink's own record.
func TestInMemorySink_EventsReturnsACopy(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	require.NoError(t, sink.Emit(t.Context(), Event{Event: "original"}))

	got := sink.Events()
	got[0].Event = "rewritten"

	assert.Equal(t, EventType("original"), sink.Events()[0].Event)
}

// The tool loop emits from several goroutines, so a sink that is not safe under
// concurrency corrupts a turn rather than merely racing a test.
func TestInMemorySink_ConcurrentEmitIsSafe(t *testing.T) {
	t.Parallel()

	const emitters = 32

	sink := NewInMemorySink()

	var wg sync.WaitGroup

	wg.Add(emitters)

	for range emitters {
		go func() {
			defer wg.Done()

			_ = sink.Emit(t.Context(), Event{Event: "concurrent"})
		}()
	}

	wg.Wait()

	assert.Equal(t, emitters, sink.Len())
}

func TestSliceSource_YieldsInOrderThenEnds(t *testing.T) {
	t.Parallel()

	src := NewSliceSource([]Event{{Event: "a"}, {Event: "b"}})

	first, err := src.Next(t.Context())
	require.NoError(t, err)
	assert.Equal(t, EventType("a"), first.Event)

	second, err := src.Next(t.Context())
	require.NoError(t, err)
	assert.Equal(t, EventType("b"), second.Event)

	_, err = src.Next(t.Context())
	require.ErrorIs(t, err, ErrNoMoreEvents)
}

func TestSliceSource_EmptyEndsImmediately(t *testing.T) {
	t.Parallel()

	_, err := NewSliceSource(nil).Next(t.Context())

	require.ErrorIs(t, err, ErrNoMoreEvents)
}

// The constructor copies, so a caller reusing its slice cannot rewrite a replay
// already in progress.
func TestSliceSource_CopiesTheCallersSlice(t *testing.T) {
	t.Parallel()

	events := []Event{{Event: "original"}}
	src := NewSliceSource(events)

	events[0].Event = "mutated"

	got, err := src.Next(t.Context())
	require.NoError(t, err)
	assert.Equal(t, EventType("original"), got.Event)
}

// The round trip every binding is measured against: what a sink collected must
// replay through a Source unchanged.
func TestInMemorySink_RoundTripsThroughSliceSource(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	pub := NewPublisher(t.Context(), sink)

	require.NoError(t, pub.SendStreamPreamble("m1", "c1", "some-model"))
	require.NoError(t, pub.SendStreamEpilogue(StopReasonEndTurn, 3))

	src := NewSliceSource(sink.Events())

	var replayed []EventType

	for {
		ev, err := src.Next(t.Context())
		if errors.Is(err, ErrNoMoreEvents) {
			break
		}

		require.NoError(t, err)

		replayed = append(replayed, ev.Event)
	}

	assert.Equal(t, []EventType{
		EventTypeMessageStart,
		EventTypePing,
		EventTypeMessageDelta,
		EventTypeMessageStop,
	}, replayed)
}
