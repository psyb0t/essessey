package nats

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

// testEventTypePing is reused across test cases below — with no subject
// prefix configured, the subject IS the bare event type, so the same
// constant doubles as both the event name and the expected subject.
const testEventTypePing essessey.EventType = "ping"

// errBoom is a static sentinel standing in for whatever a real *nats.Conn
// might return from Publish.
var errBoom = errors.New("boom")

// fakePublisher implements Publisher without any real NATS connection.
type fakePublisher struct {
	mu         sync.Mutex
	subject    string
	data       []byte
	calls      int
	publishErr error
}

func (f *fakePublisher) Publish(subject string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.subject = subject

	f.data = append([]byte(nil), data...)

	return f.publishErr
}

func TestSink_Emit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		subjectPrefix string
		event         essessey.EventType
		data          json.RawMessage
		wantSubject   string
	}{
		{
			name:          "with prefix",
			subjectPrefix: "chat",
			event:         "message_start",
			data:          json.RawMessage(`{"a":1}`),
			wantSubject:   "chat.message_start",
		},
		{
			name:          "without prefix",
			subjectPrefix: "",
			event:         testEventTypePing,
			data:          json.RawMessage(`{}`),
			wantSubject:   testEventTypePing,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pub := &fakePublisher{}
			sink := NewSink(pub, tc.subjectPrefix)

			err := sink.Emit(context.Background(), essessey.Event{
				Event: tc.event,
				Data:  tc.data,
			})
			require.NoError(t, err)

			assert.Equal(t, 1, pub.calls)
			assert.Equal(t, tc.wantSubject, pub.subject)
			assert.Equal(t, []byte(tc.data), pub.data)
		})
	}
}

func TestSink_Emit_PublishError(t *testing.T) {
	t.Parallel()

	pub := &fakePublisher{publishErr: errBoom}
	sink := NewSink(pub, "chat")

	err := sink.Emit(context.Background(), essessey.Event{
		Event: testEventTypePing,
		Data:  json.RawMessage(`{}`),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}
