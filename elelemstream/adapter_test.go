package elelemstream

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/psyb0t/elelem"
	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCallID1 = "call_1"
	testCallIDA = "call_a"
	testCallIDB = "call_b"

	testToolNameWeather = "get_weather"
	testToolNameA       = "tool_a"
	testToolNameB       = "tool_b"
)

// wireEvent is the shape shared by every content_block_* payload this
// package emits: an index, plus a type tag under content_block for a start
// event or under delta for a delta event. Decoding into one loose struct
// keeps the tests from caring which concrete essessey type produced it.
//
//nolint:tagliatelle // snake_case matches essessey's wire format
type wireEvent struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
	} `json:"content_block"`
	Delta struct {
		Type string `json:"type"`
	} `json:"delta"`
}

func decodeEvents(t *testing.T, events []essessey.Event) []wireEvent {
	t.Helper()

	decoded := make([]wireEvent, len(events))

	for i, ev := range events {
		var we wireEvent
		require.NoError(t, json.Unmarshal(ev.Data, &we), "event %d", i)
		decoded[i] = we
	}

	return decoded
}

// blockType returns the type this event names, whichever field carries it.
func (we wireEvent) blockType() string {
	if we.ContentBlock.Type != "" {
		return we.ContentBlock.Type
	}

	return we.Delta.Type
}

func newTestAdapter() (*Adapter, *essessey.InMemorySink) {
	sink := essessey.NewInMemorySink()
	pub := essessey.NewPublisher(context.Background(), sink)

	return New(pub), sink
}

func TestAdapter_TextOnlyRound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	adapter, sink := newTestAdapter()

	require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{Round: 0}))
	require.NoError(t, adapter.onDelta(ctx, elelem.Delta{Text: "hello"}))
	require.NoError(t, adapter.onAssistantMessage(ctx, elelem.Message{
		Role:    elelem.RoleAssistant,
		Content: elelem.Text("hello"),
	}))
	require.NoError(t, adapter.onRoundEnd(ctx, &elelem.RoundEvent{Round: 0}))

	events := decodeEvents(t, sink.Events())
	require.Len(t, events, 3)

	for i, ev := range events {
		assert.Equal(t, 0, ev.Index, "event %d index", i)
	}

	assert.Equal(t, essessey.ContentBlockTypeText, events[0].blockType())
	assert.Equal(t, essessey.ContentBlockTypeTextDelta, events[1].blockType())
	assert.Equal(t, 1, adapter.Rounds())
}

func TestAdapter_ThinkingThenText(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	adapter, sink := newTestAdapter()

	require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{Round: 0}))
	require.NoError(t, adapter.onDelta(
		ctx, elelem.Delta{Reasoning: "thinking..."},
	))
	require.NoError(t, adapter.onDelta(ctx, elelem.Delta{Text: "answer"}))
	require.NoError(t, adapter.onAssistantMessage(ctx, elelem.Message{
		Role:    elelem.RoleAssistant,
		Content: elelem.Text("answer"),
	}))
	require.NoError(t, adapter.onRoundEnd(ctx, &elelem.RoundEvent{Round: 0}))

	events := decodeEvents(t, sink.Events())
	// thinking: start + delta + stop (index 0), text: start + delta + stop
	// (index 1).
	require.Len(t, events, 6)

	thinkingIndices := []int{events[0].Index, events[1].Index, events[2].Index}
	assert.Equal(t, []int{0, 0, 0}, thinkingIndices, "thinking block indices")

	textIndices := []int{events[3].Index, events[4].Index, events[5].Index}
	assert.Equal(t, []int{1, 1, 1}, textIndices, "text block indices")

	assert.Equal(t, essessey.ContentBlockTypeThinking, events[0].blockType())
	assert.Equal(t, essessey.ContentBlockTypeText, events[3].blockType())
}

func TestAdapter_OneToolCall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	adapter, sink := newTestAdapter()

	require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{Round: 0}))
	require.NoError(t, adapter.onAssistantMessage(ctx, elelem.Message{
		Role: elelem.RoleAssistant,
		ToolCalls: []elelem.ToolCall{
			{
				ID: testCallID1, Name: testToolNameWeather,
				Arguments: json.RawMessage(`{}`),
			},
		},
	}))
	require.NoError(t, adapter.onToolCallStart(ctx, elelem.ToolCallEvent{
		CallID:    testCallID1,
		Name:      testToolNameWeather,
		Arguments: json.RawMessage(`{"city":"NYC"}`),
		Index:     0,
	}))
	require.NoError(t, adapter.onToolResult(ctx, elelem.ToolCallEvent{
		CallID:    testCallID1,
		Name:      testToolNameWeather,
		Arguments: json.RawMessage(`{"city":"NYC"}`),
		Index:     0,
		Result:    &elelem.ToolResult{Content: "sunny"},
	}))
	require.NoError(t, adapter.onRoundEnd(ctx, &elelem.RoundEvent{Round: 0}))

	events := decodeEvents(t, sink.Events())
	// tool_use: start + delta + stop (index 0), tool_result: start + delta +
	// stop (index 1).
	require.Len(t, events, 6)

	for i := range 3 {
		assert.Equal(t, 0, events[i].Index, "tool_use event %d", i)
	}

	for i := 3; i < 6; i++ {
		assert.Equal(t, 1, events[i].Index, "tool_result event %d", i)
	}

	assert.Equal(t, essessey.ContentBlockTypeToolUse, events[0].blockType())
	assert.Equal(t, essessey.ContentBlockTypeToolResult, events[3].blockType())
}

func TestAdapter_TwoParallelToolCalls_ThenSecondRound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	adapter, sink := newTestAdapter()

	// Round 0: two parallel tool calls, no text/thinking content.
	require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{Round: 0}))
	require.NoError(t, adapter.onAssistantMessage(ctx, elelem.Message{
		Role: elelem.RoleAssistant,
		ToolCalls: []elelem.ToolCall{
			{
				ID: testCallIDA, Name: testToolNameA,
				Arguments: json.RawMessage(`{}`),
			},
			{
				ID: testCallIDB, Name: testToolNameB,
				Arguments: json.RawMessage(`{}`),
			},
		},
	}))
	require.NoError(t, adapter.onToolCallStart(ctx, elelem.ToolCallEvent{
		CallID: testCallIDA, Name: testToolNameA, Index: 0,
	}))
	require.NoError(t, adapter.onToolCallStart(ctx, elelem.ToolCallEvent{
		CallID: testCallIDB, Name: testToolNameB, Index: 1,
	}))
	require.NoError(t, adapter.onToolResult(ctx, elelem.ToolCallEvent{
		CallID: testCallIDA, Name: testToolNameA, Index: 0,
		Result: &elelem.ToolResult{Content: "result a"},
	}))
	require.NoError(t, adapter.onToolResult(ctx, elelem.ToolCallEvent{
		CallID: testCallIDB, Name: testToolNameB, Index: 1,
		Result: &elelem.ToolResult{Content: "result b"},
	}))
	require.NoError(t, adapter.onRoundEnd(ctx, &elelem.RoundEvent{Round: 0}))

	events := decodeEvents(t, sink.Events())
	require.Len(t, events, 12)

	toolUseAIndices := []int{
		events[0].Index, events[1].Index, events[2].Index,
	}
	assert.Equal(t, []int{0, 0, 0}, toolUseAIndices, "tool_use call_a")

	toolUseBIndices := []int{
		events[3].Index, events[4].Index, events[5].Index,
	}
	assert.Equal(t, []int{1, 1, 1}, toolUseBIndices, "tool_use call_b")

	toolResultAIndices := []int{
		events[6].Index, events[7].Index, events[8].Index,
	}
	assert.Equal(t, []int{2, 2, 2}, toolResultAIndices, "tool_result call_a")

	toolResultBIndices := []int{
		events[9].Index, events[10].Index, events[11].Index,
	}
	assert.Equal(t, []int{3, 3, 3}, toolResultBIndices, "tool_result call_b")

	// Round 1 starts at the advanced index (toolBase=0, toolCallCount=2 =>
	// blockIndex = 0 + 2*2 = 4), not back at 0 and not still at 1.
	require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{Round: 1}))
	require.NoError(t, adapter.onDelta(ctx, elelem.Delta{Text: "final answer"}))
	require.NoError(t, adapter.onAssistantMessage(ctx, elelem.Message{
		Role:    elelem.RoleAssistant,
		Content: elelem.Text("final answer"),
	}))
	require.NoError(t, adapter.onRoundEnd(ctx, &elelem.RoundEvent{Round: 1}))

	allEvents := decodeEvents(t, sink.Events())
	require.Len(t, allEvents, 15)

	secondRoundIndices := []int{
		allEvents[12].Index, allEvents[13].Index, allEvents[14].Index,
	}
	assert.Equal(
		t, []int{4, 4, 4}, secondRoundIndices, "second round text block",
	)
	assert.Equal(
		t, essessey.ContentBlockTypeText, allEvents[12].blockType(),
	)
	assert.Equal(t, 2, adapter.Rounds())
}

// TestAdapter_TextThenParallelToolCalls_NonZeroToolBase drives a round
// that streams text BEFORE making parallel tool calls, so toolBase lands
// on a non-zero value (1). Every other tool-call test in this file starts
// its round directly with tool calls, leaving toolBase == 0 and the
// a.toolBase+ offset in onToolCallStart/onToolResult unverified.
func TestAdapter_TextThenParallelToolCalls_NonZeroToolBase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	adapter, sink := newTestAdapter()

	// Round 0: text first (toolBase becomes 1 once it closes), then two
	// parallel tool calls.
	require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{Round: 0}))
	require.NoError(t, adapter.onDelta(
		ctx, elelem.Delta{Text: "let me check"},
	))
	require.NoError(t, adapter.onAssistantMessage(ctx, elelem.Message{
		Role:    elelem.RoleAssistant,
		Content: elelem.Text("let me check"),
		ToolCalls: []elelem.ToolCall{
			{
				ID: testCallIDA, Name: testToolNameA,
				Arguments: json.RawMessage(`{}`),
			},
			{
				ID: testCallIDB, Name: testToolNameB,
				Arguments: json.RawMessage(`{}`),
			},
		},
	}))
	require.NoError(t, adapter.onToolCallStart(ctx, elelem.ToolCallEvent{
		CallID: testCallIDA, Name: testToolNameA, Index: 0,
	}))
	require.NoError(t, adapter.onToolCallStart(ctx, elelem.ToolCallEvent{
		CallID: testCallIDB, Name: testToolNameB, Index: 1,
	}))
	require.NoError(t, adapter.onToolResult(ctx, elelem.ToolCallEvent{
		CallID: testCallIDA, Name: testToolNameA, Index: 0,
		Result: &elelem.ToolResult{Content: "result a"},
	}))
	require.NoError(t, adapter.onToolResult(ctx, elelem.ToolCallEvent{
		CallID: testCallIDB, Name: testToolNameB, Index: 1,
		Result: &elelem.ToolResult{Content: "result b"},
	}))
	require.NoError(t, adapter.onRoundEnd(ctx, &elelem.RoundEvent{Round: 0}))

	// Round 1: text only, confirming blockIndex advanced past both tool
	// blocks (toolBase 1 + 2*toolCallCount 2 == 5), not back to 0 or 1.
	require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{Round: 1}))
	require.NoError(t, adapter.onDelta(ctx, elelem.Delta{Text: "done"}))
	require.NoError(t, adapter.onAssistantMessage(ctx, elelem.Message{
		Role:    elelem.RoleAssistant,
		Content: elelem.Text("done"),
	}))
	require.NoError(t, adapter.onRoundEnd(ctx, &elelem.RoundEvent{Round: 1}))

	events := decodeEvents(t, sink.Events())
	require.Len(t, events, 18)

	wantIndices := []int{
		0, 0, 0, // round 0 text block
		1, 1, 1, // tool_use call_a (toolBase 1 + index 0)
		2, 2, 2, // tool_use call_b (toolBase 1 + index 1)
		3, 3, 3, // tool_result call_a (toolBase 1 + count 2 + index 0)
		4, 4, 4, // tool_result call_b (toolBase 1 + count 2 + index 1)
		5, 5, 5, // round 1 text block (toolBase 1 + 2*count 2)
	}
	for i, want := range wantIndices {
		assert.Equal(t, want, events[i].Index, "event %d index", i)
	}

	assert.Equal(t, essessey.ContentBlockTypeText, events[0].blockType())
	assert.Equal(t, essessey.ContentBlockTypeToolUse, events[3].blockType())
	assert.Equal(t, essessey.ContentBlockTypeToolUse, events[6].blockType())
	assert.Equal(
		t, essessey.ContentBlockTypeToolResult, events[9].blockType(),
	)
	assert.Equal(
		t, essessey.ContentBlockTypeToolResult, events[12].blockType(),
	)
	assert.Equal(t, essessey.ContentBlockTypeText, events[15].blockType())
}

func TestAdapter_Bind(t *testing.T) {
	t.Parallel()

	adapter, _ := newTestAdapter()

	client := elelem.New(nil)
	req := adapter.Bind(elelem.NewRequest(client))

	require.NotNil(t, req)
}

func TestAdapter_UninitializedRoundStream(t *testing.T) {
	t.Parallel()

	t.Run("onDelta before onRoundStart", func(t *testing.T) {
		t.Parallel()

		adapter, _ := newTestAdapter()
		err := adapter.onDelta(context.Background(), elelem.Delta{Text: "x"})
		require.Error(t, err)
	})

	t.Run("onAssistantMessage before onRoundStart", func(t *testing.T) {
		t.Parallel()

		adapter, _ := newTestAdapter()
		err := adapter.onAssistantMessage(context.Background(), elelem.Message{
			Role: elelem.RoleAssistant,
		})
		require.Error(t, err)
	})
}

func TestAdapter_OnToolResult_MissingResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	adapter, _ := newTestAdapter()

	err := adapter.onToolResult(ctx, elelem.ToolCallEvent{
		CallID: testCallID1, Name: testToolNameWeather, Index: 0,
	})
	require.Error(t, err)
}

// errSink always fails Emit, so tests can assert that a Publisher failure
// comes back wrapped rather than swallowed.
type errSink struct{}

func (errSink) Emit(_ context.Context, _ essessey.Event) error {
	return assert.AnError
}

func TestAdapter_PublishErrors_AreWrapped(t *testing.T) {
	t.Parallel()

	t.Run("onDelta text write failure", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		pub := essessey.NewPublisher(ctx, errSink{})
		adapter := New(pub)
		require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{}))

		err := adapter.onDelta(ctx, elelem.Delta{Text: "hello"})
		require.Error(t, err)
	})

	t.Run("onDelta reasoning write failure", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		pub := essessey.NewPublisher(ctx, errSink{})
		adapter := New(pub)
		require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{}))

		err := adapter.onDelta(ctx, elelem.Delta{Reasoning: "thinking"})
		require.Error(t, err)
	})

	t.Run("empty round publishes nothing", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		pub := essessey.NewPublisher(ctx, errSink{})
		adapter := New(pub)
		require.NoError(t, adapter.onRoundStart(ctx, &elelem.RoundEvent{}))

		// No content streamed this round, so finish() never opens a block —
		// blocks open lazily, on first content — and the failing sink is
		// never even called.
		err := adapter.onAssistantMessage(ctx, elelem.Message{
			Role: elelem.RoleAssistant,
			ToolCalls: []elelem.ToolCall{
				{ID: testCallID1, Name: testToolNameWeather},
			},
		})
		require.NoError(t, err)
	})

	t.Run("onToolCallStart failure", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		pub := essessey.NewPublisher(ctx, errSink{})
		adapter := New(pub)

		err := adapter.onToolCallStart(ctx, elelem.ToolCallEvent{
			CallID: testCallID1, Name: testToolNameWeather, Index: 0,
		})
		require.Error(t, err)
	})

	t.Run("onToolResult failure", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		pub := essessey.NewPublisher(ctx, errSink{})
		adapter := New(pub)

		err := adapter.onToolResult(ctx, elelem.ToolCallEvent{
			CallID: testCallID1, Name: testToolNameWeather, Index: 0,
			Result: &elelem.ToolResult{Content: "sunny"},
		})
		require.Error(t, err)
	})
}
