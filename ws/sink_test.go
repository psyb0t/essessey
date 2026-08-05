package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEventTypePing is reused across the test cases below.
const testEventTypePing essessey.EventType = "ping"

// errBoom is a static sentinel standing in for whatever a real
// *websocket.Conn might return from WriteJSON.
var errBoom = errors.New("boom")

// fakeConn implements Conn without any real WebSocket connection. It has
// no mutex of its own — Sink's own locking is what must keep concurrent
// Emit calls race-free.
type fakeConn struct {
	writes   []essessey.Event
	writeErr error
}

func (f *fakeConn) WriteJSON(v any) error {
	if f.writeErr != nil {
		return f.writeErr
	}

	ev, _ := v.(essessey.Event)
	f.writes = append(f.writes, ev)

	return nil
}

func TestSink_Emit(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{}
	sink := NewSink(conn)

	ev := essessey.Event{
		Event: "message_start",
		Data:  json.RawMessage(`{"a":1}`),
	}

	err := sink.Emit(context.Background(), ev)
	require.NoError(t, err)

	require.Len(t, conn.writes, 1)
	assert.Equal(t, ev, conn.writes[0])
}

func TestSink_Emit_WriteError(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{writeErr: errBoom}
	sink := NewSink(conn)

	err := sink.Emit(
		context.Background(),
		essessey.Event{Event: testEventTypePing},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}

func TestSink_Emit_ConcurrentIsRaceFree(t *testing.T) {
	t.Parallel()

	conn := &fakeConn{}
	sink := NewSink(conn)

	const n = 50

	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()

			ev := essessey.Event{
				Event: testEventTypePing,
				Data:  json.RawMessage(`{}`),
			}
			assert.NoError(t, sink.Emit(context.Background(), ev))
		}()
	}

	wg.Wait()

	assert.Len(t, conn.writes, n)
}
