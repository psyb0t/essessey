package essessey

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingSink reports a delivery failure, so the wrap path is exercised.
type failingSink struct{ err error }

func (s failingSink) Emit(context.Context, Event) error {
	return s.err
}

func TestPublisher_PublishEmitsNameAndJSONPayload(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	pub := NewPublisher(t.Context(), sink)

	err := pub.Publish(EventTypePing, PingData{Type: EventTypePing})
	require.NoError(t, err)

	events := sink.Events()
	require.Len(t, events, 1)
	assert.Equal(t, EventTypePing, events[0].Event)
	assert.JSONEq(t, `{"type":"ping"}`, string(events[0].Data))
}

// A payload that cannot marshal must fail BEFORE anything reaches the sink —
// emitting a half-built event would put a malformed frame on the wire.
func TestPublisher_PublishRejectsUnmarshalablePayload(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	pub := NewPublisher(t.Context(), sink)

	err := pub.Publish(EventTypePing, make(chan int))

	require.Error(t, err)
	assert.Zero(t, sink.Len(), "nothing may be emitted when marshal fails")
}

func TestPublisher_PublishWrapsSinkFailure(t *testing.T) {
	t.Parallel()

	sentinel := ErrNoMoreEvents // any error; identity is what matters here
	pub := NewPublisher(t.Context(), failingSink{err: sentinel})

	err := pub.Publish(EventTypePing, PingData{Type: EventTypePing})

	require.ErrorIs(t, err, sentinel)
}

// Every Send* helper is a thin shell over Publish; what matters is that each
// one names the right event and tags the right block type, because the client
// dispatches on exactly those two strings.
func TestPublisher_SendHelpersEmitTheRightEventAndBlockType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		send      func(*Publisher) error
		wantEvent EventType
		wantType  ContentBlockType
	}{
		{
			"text block start",
			func(p *Publisher) error {
				return p.SendContentBlockStartText(0)
			},
			EventTypeContentBlockStart,
			ContentBlockTypeText,
		},
		{
			"text delta",
			func(p *Publisher) error {
				return p.SendContentBlockDeltaText(0, "hi")
			},
			EventTypeContentBlockDelta,
			ContentBlockTypeTextDelta,
		},
		{
			"thinking block start",
			func(p *Publisher) error {
				return p.SendContentBlockStartThinking(0)
			},
			EventTypeContentBlockStart,
			ContentBlockTypeThinking,
		},
		{
			"thinking delta",
			func(p *Publisher) error {
				return p.SendContentBlockDeltaThinking(0, "hm")
			},
			EventTypeContentBlockDelta,
			ContentBlockTypeThinkingDelta,
		},
		{
			"tool use start",
			func(p *Publisher) error {
				return p.SendToolUseStart(0, "call-1", "lookup")
			},
			EventTypeContentBlockStart,
			ContentBlockTypeToolUse,
		},
		{
			"tool input delta",
			func(p *Publisher) error {
				return p.SendToolInputDelta(0, "{}")
			},
			EventTypeContentBlockDelta,
			ContentBlockTypeInputJSON,
		},
		{
			"tool result start",
			func(p *Publisher) error {
				return p.SendToolResultStart(0, "call-1", false)
			},
			EventTypeContentBlockStart,
			ContentBlockTypeToolResult,
		},
		{
			"tool result delta",
			func(p *Publisher) error {
				return p.SendToolResultDelta(0, "done")
			},
			EventTypeContentBlockDelta,
			ContentBlockTypeJSONPartial,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := NewInMemorySink()
			require.NoError(t, tc.send(NewPublisher(t.Context(), sink)))

			events := sink.Events()
			require.Len(t, events, 1)
			assert.Equal(t, tc.wantEvent, events[0].Event)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(events[0].Data, &payload))

			assert.Equal(t, tc.wantType, nestedType(t, payload))
		})
	}
}

// nestedType digs the block type out of whichever nested object carries it —
// content_block on a start, delta on a delta.
func nestedType(t *testing.T, payload map[string]any) string {
	t.Helper()

	for _, key := range []string{"content_block", "delta"} {
		nested, ok := payload[key].(map[string]any)
		if !ok {
			continue
		}

		blockType, ok := nested["type"].(string)
		require.True(t, ok, "nested %q has no string type", key)

		return blockType
	}

	t.Fatalf("payload carries neither content_block nor delta: %v", payload)

	return ""
}

// The composite helpers are the ones a caller reaches for; each must produce a
// COMPLETE block, because a start without its stop leaves the client rendering
// an open card forever.
func TestPublisher_CompositeHelpersEmitWholeBlocks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		send       func(*Publisher) error
		wantEvents []EventType
	}{
		{
			"stream preamble",
			func(p *Publisher) error {
				return p.SendStreamPreamble("m1", "c1", "some-model")
			},
			[]EventType{EventTypeMessageStart, EventTypePing},
		},
		{
			"stream epilogue",
			func(p *Publisher) error {
				return p.SendStreamEpilogue(StopReasonEndTurn, 12)
			},
			[]EventType{EventTypeMessageDelta, EventTypeMessageStop},
		},
		{
			"tool use block",
			func(p *Publisher) error {
				return p.SendToolUseBlock(3, "call-1", "lookup", "{}")
			},
			[]EventType{
				EventTypeContentBlockStart,
				EventTypeContentBlockDelta,
				EventTypeContentBlockStop,
			},
		},
		{
			"tool result block",
			func(p *Publisher) error {
				return p.SendToolResultBlock(4, "call-1", "ok", false)
			},
			[]EventType{
				EventTypeContentBlockStart,
				EventTypeContentBlockDelta,
				EventTypeContentBlockStop,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := NewInMemorySink()
			require.NoError(t, tc.send(NewPublisher(t.Context(), sink)))

			emitted := sink.Events()
			got := make([]EventType, 0, len(emitted))

			for _, ev := range emitted {
				got = append(got, ev.Event)
			}

			assert.Equal(t, tc.wantEvents, got)
		})
	}
}

// message_start carries the conversation id because that is how a client learns
// a freshly-created conversation's id — from the stream, not a second request.
func TestPublisher_MessageStartCarriesConversationID(t *testing.T) {
	t.Parallel()

	sink := NewInMemorySink()
	pub := NewPublisher(t.Context(), sink)

	require.NoError(t, pub.SendMessageStart("m1", "conv-42", "some-model"))

	var payload MessageStartData
	require.NoError(t, json.Unmarshal(sink.Events()[0].Data, &payload))

	assert.Equal(t, "conv-42", payload.Message.ConversationID)
	assert.Equal(t, "m1", payload.Message.ID)
	assert.Equal(t, RoleAssistant, payload.Message.Role)
}
