package sse

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterSink_Emit_RoundTripsThroughSource(t *testing.T) {
	t.Parallel()

	want := []essessey.Event{
		{
			Event: essessey.EventTypeMessageStart,
			Data:  json.RawMessage(`{"id":"msg-1"}`),
		},
		{
			Event: essessey.EventTypeContentBlockDelta,
			Data:  json.RawMessage(`{"text":"hi"}`),
		},
		{
			Event: essessey.EventTypeMessageStop,
			Data:  json.RawMessage(`{}`),
		},
	}

	var buf bytes.Buffer

	sink := NewWriterSink(&buf)

	for _, ev := range want {
		require.NoError(t, sink.Emit(t.Context(), ev))
	}

	source := NewSource(&buf)

	var got []essessey.Event

	for {
		ev, err := source.Next(t.Context())
		if errors.Is(err, essessey.ErrNoMoreEvents) {
			break
		}

		require.NoError(t, err)

		got = append(got, ev)
	}

	assert.Equal(t, want, got)
}

func TestWriterSink_Emit_ConcurrentIsRaceFree(t *testing.T) {
	t.Parallel()

	const goroutines = 20

	var buf bytes.Buffer

	sink := NewWriterSink(&buf)

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			ev := essessey.Event{
				Event: essessey.EventTypePing,
				Data:  json.RawMessage(`{}`),
			}
			assert.NoError(t, sink.Emit(t.Context(), ev))
		}()
	}

	wg.Wait()

	source := NewSource(&buf)

	count := 0

	for {
		_, err := source.Next(t.Context())
		if errors.Is(err, essessey.ErrNoMoreEvents) {
			break
		}

		require.NoError(t, err)

		count++
	}

	assert.Equal(t, goroutines, count)
}

// nonFlushingWriter implements http.ResponseWriter but deliberately omits
// Flush, so NewHTTPSink must reject it.
type nonFlushingWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newNonFlushingWriter() *nonFlushingWriter {
	return &nonFlushingWriter{header: make(http.Header)}
}

func (w *nonFlushingWriter) Header() http.Header {
	return w.header
}

func (w *nonFlushingWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *nonFlushingWriter) WriteHeader(status int) {
	w.status = status
}

func TestNewHTTPSink_RejectsNonFlusher(t *testing.T) {
	t.Parallel()

	sink, err := NewHTTPSink(newNonFlushingWriter())

	require.Error(t, err)
	assert.Nil(t, sink)
}

func TestHTTPSink_Emit_Flushes(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	sink, err := NewHTTPSink(rec)
	require.NoError(t, err)

	ev := essessey.Event{
		Event: essessey.EventTypePing,
		Data:  json.RawMessage(`{"type":"ping"}`),
	}

	require.NoError(t, sink.Emit(t.Context(), ev))

	assert.True(t, rec.Flushed)
	assert.Equal(t, FrameLines(ev), rec.Body.String())
}
