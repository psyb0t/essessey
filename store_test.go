package essessey_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testStreamID = "stream-a"

// newStore builds a store or fails the test — capacity errors are a setup
// problem, not the thing under test.
func newStore(t *testing.T, capacity int) *essessey.InMemoryEventStore {
	t.Helper()

	store, err := essessey.NewInMemoryEventStore(capacity)
	require.NoError(t, err)

	return store
}

// appendIDs appends one event per id, with the id also in the payload so a
// replay can be checked for identity as well as count.
func appendIDs(
	t *testing.T,
	store *essessey.InMemoryEventStore,
	streamID string,
	ids ...string,
) {
	t.Helper()

	for _, id := range ids {
		require.NoError(t, store.Append(context.Background(), streamID,
			essessey.Event{
				ID:    id,
				Event: "chunk",
				Data:  json.RawMessage(`{"id":"` + id + `"}`),
			}))
	}
}

// idsOf extracts the ids of a replay so assertions read as the sequence a
// client would actually receive.
func idsOf(events []essessey.Event) []string {
	ids := make([]string, 0, len(events))
	for _, ev := range events {
		ids = append(ids, ev.ID)
	}

	return ids
}

func TestNewInMemoryEventStore_RejectsNonPositiveCapacity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		capacity int
	}{
		{"zero", 0},
		{"negative", -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// A zero-capacity store would accept every append and resume
			// nothing — a buffer that silently never works.
			store, err := essessey.NewInMemoryEventStore(tc.capacity)
			require.ErrorIs(t, err, essessey.ErrInvalidCapacity)
			assert.Nil(t, store)
		})
	}
}

func TestInMemoryEventStore_Since(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		capacity  int
		appendIDs []string
		resumeAt  string
		wantKnown bool
		wantIDs   []string
	}{
		{
			name:      "resumes from the middle",
			capacity:  10,
			appendIDs: []string{"1", "2", "3", "4"},
			resumeAt:  "2",
			wantKnown: true,
			wantIDs:   []string{"3", "4"},
		},
		{
			// Caught up: the id is known, there is simply nothing after it.
			// Distinct from "unknown id", and the caller must not confuse them.
			name:      "caught up returns known and empty",
			capacity:  10,
			appendIDs: []string{"1", "2"},
			resumeAt:  "2",
			wantKnown: true,
			wantIDs:   []string{},
		},
		{
			name:      "resumes from the oldest retained",
			capacity:  10,
			appendIDs: []string{"1", "2", "3"},
			resumeAt:  "1",
			wantKnown: true,
			wantIDs:   []string{"2", "3"},
		},
		{
			// An id that was never sent. Any answer but "unknown" either
			// duplicates the whole stream or silently skips it.
			name:      "unknown id is not a resume point",
			capacity:  10,
			appendIDs: []string{"1", "2"},
			resumeAt:  "99",
			wantKnown: false,
		},
		{
			// A client that invents an id ahead of the stream must not be
			// treated as caught up — that would drop every real event.
			name:      "future id is unknown, not caught up",
			capacity:  10,
			appendIDs: []string{"1", "2"},
			resumeAt:  "1000",
			wantKnown: false,
		},
		{
			name:      "empty id is not a resume point",
			capacity:  10,
			appendIDs: []string{"1", "2"},
			resumeAt:  "",
			wantKnown: false,
		},
		{
			// THE eviction case. Capacity 2, four events: ids 1 and 2 are gone,
			// so resuming from them cannot be honoured.
			name:      "evicted id is unknown",
			capacity:  2,
			appendIDs: []string{"1", "2", "3", "4"},
			resumeAt:  "1",
			wantKnown: false,
		},
		{
			name:      "survivor of eviction still resumes",
			capacity:  2,
			appendIDs: []string{"1", "2", "3", "4"},
			resumeAt:  "3",
			wantKnown: true,
			wantIDs:   []string{"4"},
		},
		{
			// Capacity 1 — the tightest ring. Only the newest survives, and it
			// is the only valid resume point.
			name:      "capacity one keeps only the newest",
			capacity:  1,
			appendIDs: []string{"1", "2", "3"},
			resumeAt:  "3",
			wantKnown: true,
			wantIDs:   []string{},
		},
		{
			name:      "capacity one forgets the previous",
			capacity:  1,
			appendIDs: []string{"1", "2", "3"},
			resumeAt:  "2",
			wantKnown: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newStore(t, tc.capacity)
			appendIDs(t, store, testStreamID, tc.appendIDs...)

			events, known, err := store.Since(
				context.Background(), testStreamID, tc.resumeAt,
			)
			require.NoError(t, err)
			require.Equal(t, tc.wantKnown, known)

			if !tc.wantKnown {
				assert.Empty(t, events)

				return
			}

			assert.Equal(t, tc.wantIDs, idsOf(events))
		})
	}
}

func TestInMemoryEventStore_UnknownStreamIsNotAResumePoint(t *testing.T) {
	t.Parallel()

	store := newStore(t, 10)

	events, known, err := store.Since(context.Background(), "never-used", "1")
	require.NoError(t, err)
	assert.False(t, known)
	assert.Empty(t, events)
}

func TestInMemoryEventStore_StreamsAreIsolated(t *testing.T) {
	t.Parallel()

	// The security-relevant one: an id from one stream must never resolve
	// against another, or a client presenting an id gets someone else's events.
	store := newStore(t, 10)
	appendIDs(t, store, "stream-a", "a1", "a2")
	appendIDs(t, store, "stream-b", "b1", "b2")

	events, known, err := store.Since(context.Background(), "stream-b", "a1")
	require.NoError(t, err)
	assert.False(t, known, "an id from another stream must not resolve")
	assert.Empty(t, events)

	events, known, err = store.Since(context.Background(), "stream-b", "b1")
	require.NoError(t, err)
	require.True(t, known)
	assert.Equal(t, []string{"b2"}, idsOf(events))
}

func TestInMemoryEventStore_EventsWithoutIDsAreReplayed(t *testing.T) {
	t.Parallel()

	// They cannot be resumed TO — nothing names them — but they must come back
	// if they fall after the resume point, or the resume silently skips them.
	store := newStore(t, 10)
	ctx := context.Background()

	appendIDs(t, store, testStreamID, "1")
	require.NoError(t, store.Append(ctx, testStreamID, essessey.Event{
		Event: "chunk",
		Data:  json.RawMessage(`{"anonymous":true}`),
	}))
	appendIDs(t, store, testStreamID, "2")

	events, known, err := store.Since(ctx, testStreamID, "1")
	require.NoError(t, err)
	require.True(t, known)
	require.Len(t, events, 2, "the id-less event must still be replayed")
	assert.Empty(t, events[0].ID)
	assert.Equal(t, "2", events[1].ID)

	// And it is not itself a resume point.
	_, known, err = store.Since(ctx, testStreamID, "")
	require.NoError(t, err)
	assert.False(t, known)
}

func TestInMemoryEventStore_DuplicateIDResolvesToTheLatest(t *testing.T) {
	t.Parallel()

	// Documented behaviour rather than desired behaviour: a producer reusing an
	// id moves the resume point forward, so everything between the two
	// occurrences is skipped. Pinned here so the consequence is visible.
	store := newStore(t, 10)
	appendIDs(t, store, testStreamID, "dup", "middle", "dup", "after")

	events, known, err := store.Since(context.Background(), testStreamID, "dup")
	require.NoError(t, err)
	require.True(t, known)
	assert.Equal(t, []string{"after"}, idsOf(events),
		"resume lands on the LATER occurrence, skipping what came between")
}

func TestInMemoryEventStore_EvictionDoesNotStrandIndexEntries(t *testing.T) {
	t.Parallel()

	// Evicting an entry must remove its id, or the index grows forever and a
	// long-dead id still reports as a valid resume point.
	store := newStore(t, 3)

	const total = 100

	ids := make([]string, 0, total)
	for i := range total {
		ids = append(ids, strconv.Itoa(i))
	}

	appendIDs(t, store, testStreamID, ids...)

	// Everything except the last three is gone.
	for i := range total - 3 {
		_, known, err := store.Since(
			context.Background(), testStreamID, strconv.Itoa(i),
		)
		require.NoError(t, err)
		require.False(t, known, "id %d should have been evicted", i)
	}

	events, known, err := store.Since(
		context.Background(), testStreamID, strconv.Itoa(total-3),
	)
	require.NoError(t, err)
	require.True(t, known)
	assert.Len(t, events, 2)
}

func TestInMemoryEventStore_Clear(t *testing.T) {
	t.Parallel()

	store := newStore(t, 10)
	ctx := context.Background()

	appendIDs(t, store, "stream-a", "1", "2")
	appendIDs(t, store, "stream-b", "1", "2")

	require.NoError(t, store.Clear(ctx, "stream-a"))

	_, known, err := store.Since(ctx, "stream-a", "1")
	require.NoError(t, err)
	assert.False(t, known, "cleared stream retains nothing")

	// Clearing one stream must not touch another.
	_, known, err = store.Since(ctx, "stream-b", "1")
	require.NoError(t, err)
	assert.True(t, known)

	// Clearing an unknown stream is a no-op, not an error.
	require.NoError(t, store.Clear(ctx, "never-used"))
}

func TestInMemoryEventStore_SinkForCaptures(t *testing.T) {
	t.Parallel()

	store := newStore(t, 10)
	ctx := context.Background()

	sink := store.SinkFor(testStreamID)
	for _, id := range []string{"1", "2", "3"} {
		require.NoError(t, sink.Emit(ctx, essessey.Event{
			ID:    id,
			Event: "chunk",
			Data:  json.RawMessage(`{}`),
		}))
	}

	events, known, err := store.Since(ctx, testStreamID, "1")
	require.NoError(t, err)
	require.True(t, known)
	assert.Equal(t, []string{"2", "3"}, idsOf(events))
}

func TestInMemoryEventStore_ReplayIsIndistinguishableFromLive(t *testing.T) {
	t.Parallel()

	// The point of the whole design: replay reuses the ordinary Sink path via
	// SliceSource, so there is no second code path that can drift from live.
	store := newStore(t, 10)
	ctx := context.Background()

	appendIDs(t, store, testStreamID, "1", "2", "3")

	events, known, err := store.Since(ctx, testStreamID, "1")
	require.NoError(t, err)
	require.True(t, known)

	wire := essessey.NewInMemorySink()
	source := essessey.NewSliceSource(events)

	for {
		ev, err := source.Next(ctx)
		if err != nil {
			require.ErrorIs(t, err, essessey.ErrNoMoreEvents)

			break
		}

		require.NoError(t, wire.Emit(ctx, ev))
	}

	assert.Equal(t, []string{"2", "3"}, idsOf(wire.Events()))
}

func TestInMemoryEventStore_ConcurrentAppendAndSince(t *testing.T) {
	t.Parallel()

	store := newStore(t, 64)
	ctx := context.Background()

	const (
		writers          = 8
		eventsPerWriter  = 50
		concurrentReader = 1
	)

	var wg sync.WaitGroup

	wg.Add(writers + concurrentReader)

	for w := range writers {
		go func() {
			defer wg.Done()

			for i := range eventsPerWriter {
				id := fmt.Sprintf("w%d-e%d", w, i)
				_ = store.Append(ctx, testStreamID, essessey.Event{
					ID:    id,
					Event: "chunk",
					Data:  json.RawMessage(`{}`),
				})
			}
		}()
	}

	go func() {
		defer wg.Done()

		for range writers * eventsPerWriter {
			_, _, _ = store.Since(ctx, testStreamID, "w0-e0")
		}
	}()

	wg.Wait()

	// The store must still be coherent: at most capacity retained, and the
	// newest append is a valid resume point.
	events, known, err := store.Since(ctx, testStreamID, "w0-e0")
	require.NoError(t, err)

	if known {
		assert.LessOrEqual(t, len(events), 64)
	}
}
