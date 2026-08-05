package essessey

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixture values reused across test cases below.
const (
	testMsgID          = "msg-1"
	testConvID         = "conv-1"
	testToolID1        = "tool-1"
	testToolID2        = "tool-2"
	testToolID9        = "tool-9"
	testToolNameSearch = "search"
	testToolNameCalc   = "calc"
)

// mustJSON marshals v and fails the test on error.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()

	b, err := json.Marshal(v)
	require.NoError(t, err)

	return b
}

func TestReassemble_TextOnly(t *testing.T) {
	t.Parallel()

	events := []Event{
		{
			Event: EventTypeMessageStart,
			Data: mustJSON(t, MessageStartData{
				Type: EventTypeMessageStart,
				Message: MessageMeta{
					ID:             testMsgID,
					ConversationID: testConvID,
					Type:           MessageTypeMessage,
					Role:           RoleAssistant,
					Model:          "test-model",
				},
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartData{
				Type:  EventTypeContentBlockStart,
				Index: 0,
				ContentBlock: ContentBlock{
					Type: ContentBlockTypeText,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaData{
				Type:  EventTypeContentBlockDelta,
				Index: 0,
				Delta: TextDelta{
					Type: ContentBlockTypeTextDelta,
					Text: "Hello ",
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaData{
				Type:  EventTypeContentBlockDelta,
				Index: 0,
				Delta: TextDelta{
					Type: ContentBlockTypeTextDelta,
					Text: "world",
				},
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 0,
			}),
		},
		{
			Event: EventTypeMessageDelta,
			Data: mustJSON(t, MessageDeltaData{
				Type: EventTypeMessageDelta,
				Delta: MessageDeltaInfo{
					StopReason: StopReasonEndTurn,
				},
				Usage: UsageEnd{OutputTokens: 2},
			}),
		},
		{
			Event: EventTypeMessageStop,
			Data: mustJSON(t, MessageStopData{
				Type: EventTypeMessageStop,
			}),
		},
	}

	got := Reassemble(context.Background(), NewSliceSource(events))

	assert.Equal(t, testConvID, got.ConversationID)
	assert.Equal(t, "Hello world", got.Text)
	assert.Empty(t, got.Error)
	assert.Empty(t, got.Tools)
	assert.Empty(t, got.Executions)

	require.Len(t, got.Timeline, 1)
	assert.Equal(t, TimelineKindText, got.Timeline[0].Kind)
	assert.Equal(t, "Hello world", got.Timeline[0].Text)
}

func TestReassemble_ToolUseAndResult(t *testing.T) {
	t.Parallel()

	events := []Event{
		{
			Event: EventTypeMessageStart,
			Data: mustJSON(t, MessageStartData{
				Type: EventTypeMessageStart,
				Message: MessageMeta{
					ID:             testMsgID,
					ConversationID: testConvID,
				},
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartToolUseData{
				Type:  EventTypeContentBlockStart,
				Index: 0,
				ContentBlock: ToolUseBlock{
					Type: ContentBlockTypeToolUse,
					ID:   testToolID1,
					Name: testToolNameSearch,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaToolInputData{
				Type:  EventTypeContentBlockDelta,
				Index: 0,
				Delta: InputJSONDelta{
					Type:        ContentBlockTypeInputJSON,
					PartialJSON: `{"q":"foo"}`,
				},
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 0,
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartToolResultData{
				Type:  EventTypeContentBlockStart,
				Index: 1,
				ContentBlock: ToolResultBlock{
					Type:      ContentBlockTypeToolResult,
					ToolUseID: testToolID1,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaToolResultData{
				Type:  EventTypeContentBlockDelta,
				Index: 1,
				Delta: ToolResultDelta{
					Type: ContentBlockTypeJSONPartial,
					Text: "42",
				},
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 1,
			}),
		},
		{
			Event: EventTypeMessageStop,
			Data: mustJSON(t, MessageStopData{
				Type: EventTypeMessageStop,
			}),
		},
	}

	got := Reassemble(context.Background(), NewSliceSource(events))

	assert.Empty(t, got.Error)
	assert.Equal(t, []string{testToolNameSearch}, got.ToolNames)

	require.Len(t, got.Tools, 1)
	assert.Equal(t, testToolNameSearch, got.Tools[0].Name)
	assert.Equal(t, testToolID1, got.Tools[0].ToolUseID)
	assert.JSONEq(t, `{"q":"foo"}`, string(got.Tools[0].Params))

	require.Len(t, got.Executions, 1)
	assert.Equal(t, testToolNameSearch, got.Executions[0].Name)
	assert.Equal(t, testToolID1, got.Executions[0].ToolUseID)
	assert.Equal(t, "42", got.Executions[0].Result)

	require.Len(t, got.Timeline, 1)
	assert.Equal(t, TimelineKindTool, got.Timeline[0].Kind)
	require.NotNil(t, got.Timeline[0].Execution)
	assert.Equal(t, testToolNameSearch, got.Timeline[0].Execution.Name)
}

func TestReassemble_ParallelToolCalls(t *testing.T) {
	t.Parallel()

	events := []Event{
		{
			Event: EventTypeMessageStart,
			Data: mustJSON(t, MessageStartData{
				Type:    EventTypeMessageStart,
				Message: MessageMeta{ID: testMsgID},
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartToolUseData{
				Type:  EventTypeContentBlockStart,
				Index: 0,
				ContentBlock: ToolUseBlock{
					Type: ContentBlockTypeToolUse,
					ID:   testToolID1,
					Name: testToolNameSearch,
				},
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartToolUseData{
				Type:  EventTypeContentBlockStart,
				Index: 1,
				ContentBlock: ToolUseBlock{
					Type: ContentBlockTypeToolUse,
					ID:   testToolID2,
					Name: testToolNameCalc,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaToolInputData{
				Type:  EventTypeContentBlockDelta,
				Index: 0,
				Delta: InputJSONDelta{
					Type:        ContentBlockTypeInputJSON,
					PartialJSON: `{"q":"a"}`,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaToolInputData{
				Type:  EventTypeContentBlockDelta,
				Index: 1,
				Delta: InputJSONDelta{
					Type:        ContentBlockTypeInputJSON,
					PartialJSON: `{"x":1}`,
				},
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 0,
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 1,
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartToolResultData{
				Type:  EventTypeContentBlockStart,
				Index: 2,
				ContentBlock: ToolResultBlock{
					Type:      ContentBlockTypeToolResult,
					ToolUseID: testToolID1,
				},
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartToolResultData{
				Type:  EventTypeContentBlockStart,
				Index: 3,
				ContentBlock: ToolResultBlock{
					Type:      ContentBlockTypeToolResult,
					ToolUseID: testToolID2,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaToolResultData{
				Type:  EventTypeContentBlockDelta,
				Index: 2,
				Delta: ToolResultDelta{
					Type: ContentBlockTypeJSONPartial,
					Text: "res1",
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaToolResultData{
				Type:  EventTypeContentBlockDelta,
				Index: 3,
				Delta: ToolResultDelta{
					Type: ContentBlockTypeJSONPartial,
					Text: "res2",
				},
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 2,
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 3,
			}),
		},
		{
			Event: EventTypeMessageStop,
			Data: mustJSON(t, MessageStopData{
				Type: EventTypeMessageStop,
			}),
		},
	}

	got := Reassemble(context.Background(), NewSliceSource(events))

	assert.Empty(t, got.Error)

	wantToolNames := []string{testToolNameSearch, testToolNameCalc}
	assert.Equal(t, wantToolNames, got.ToolNames)

	require.Len(t, got.Tools, 2)
	assert.Equal(t, testToolNameSearch, got.Tools[0].Name)
	assert.Equal(t, testToolID1, got.Tools[0].ToolUseID)
	assert.Equal(t, testToolNameCalc, got.Tools[1].Name)
	assert.Equal(t, testToolID2, got.Tools[1].ToolUseID)

	require.Len(t, got.Executions, 2)
	assert.Equal(t, testToolNameSearch, got.Executions[0].Name)
	assert.Equal(t, "res1", got.Executions[0].Result)
	assert.Equal(t, testToolNameCalc, got.Executions[1].Name)
	assert.Equal(t, "res2", got.Executions[1].Result)

	require.Len(t, got.Timeline, 2)
	assert.Equal(t, TimelineKindTool, got.Timeline[0].Kind)
	assert.Equal(t, TimelineKindTool, got.Timeline[1].Kind)
}

func TestReassemble_MalformedEventDropped(t *testing.T) {
	t.Parallel()

	events := []Event{
		{
			Event: EventTypeMessageStart,
			Data: mustJSON(t, MessageStartData{
				Type:    EventTypeMessageStart,
				Message: MessageMeta{ID: testMsgID},
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartData{
				Type:  EventTypeContentBlockStart,
				Index: 0,
				ContentBlock: ContentBlock{
					Type: ContentBlockTypeText,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaData{
				Type:  EventTypeContentBlockDelta,
				Index: 0,
				Delta: TextDelta{
					Type: ContentBlockTypeTextDelta,
					Text: "he",
				},
			}),
		},
		{
			// Malformed mid-stream event — invalid JSON. Must be
			// dropped without aborting reassembly.
			Event: EventTypeContentBlockStart,
			Data:  json.RawMessage(`{not valid json`),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaData{
				Type:  EventTypeContentBlockDelta,
				Index: 0,
				Delta: TextDelta{
					Type: ContentBlockTypeTextDelta,
					Text: "llo",
				},
			}),
		},
		{
			Event: EventTypeContentBlockStop,
			Data: mustJSON(t, ContentBlockStopData{
				Type:  EventTypeContentBlockStop,
				Index: 0,
			}),
		},
		{
			Event: EventTypeMessageStop,
			Data: mustJSON(t, MessageStopData{
				Type: EventTypeMessageStop,
			}),
		},
	}

	got := Reassemble(context.Background(), NewSliceSource(events))

	assert.Empty(t, got.Error)
	assert.Equal(t, "hello", got.Text)

	require.Len(t, got.Timeline, 1)
	assert.Equal(t, TimelineKindText, got.Timeline[0].Kind)
	assert.Equal(t, "hello", got.Timeline[0].Text)
}

func TestReassemble_EmptyStream(t *testing.T) {
	t.Parallel()

	got := Reassemble(context.Background(), NewSliceSource(nil))

	assert.Equal(t, ParsedStream{}, got)
}

func TestReassemble_NoMessageStop(t *testing.T) {
	t.Parallel()

	events := []Event{
		{
			Event: EventTypeMessageStart,
			Data: mustJSON(t, MessageStartData{
				Type: EventTypeMessageStart,
				Message: MessageMeta{
					ID:             testMsgID,
					ConversationID: testConvID,
				},
			}),
		},
		{
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartData{
				Type:  EventTypeContentBlockStart,
				Index: 0,
				ContentBlock: ContentBlock{
					Type: ContentBlockTypeText,
				},
			}),
		},
		{
			Event: EventTypeContentBlockDelta,
			Data: mustJSON(t, ContentBlockDeltaData{
				Type:  EventTypeContentBlockDelta,
				Index: 0,
				Delta: TextDelta{
					Type: ContentBlockTypeTextDelta,
					Text: "partial",
				},
			}),
		},
		{
			// tool_use opened but never closed — stream is truncated
			// before content_block_stop or message_stop arrives.
			Event: EventTypeContentBlockStart,
			Data: mustJSON(t, ContentBlockStartToolUseData{
				Type:  EventTypeContentBlockStart,
				Index: 1,
				ContentBlock: ToolUseBlock{
					Type: ContentBlockTypeToolUse,
					ID:   testToolID9,
					Name: testToolNameSearch,
				},
			}),
		},
	}

	got := Reassemble(context.Background(), NewSliceSource(events))

	assert.Empty(t, got.Error)
	assert.Equal(t, testConvID, got.ConversationID)
	assert.Equal(t, "partial", got.Text)
	assert.Equal(t, []string{testToolNameSearch}, got.ToolNames)
	assert.Empty(t, got.Executions)

	require.Len(t, got.Timeline, 1)
	assert.Equal(t, TimelineKindText, got.Timeline[0].Kind)
	assert.Equal(t, "partial", got.Timeline[0].Text)

	require.Len(t, got.Tools, 1)
	assert.Equal(t, testToolNameSearch, got.Tools[0].Name)
	assert.Equal(t, testToolID9, got.Tools[0].ToolUseID)
	assert.JSONEq(t, "{}", string(got.Tools[0].Params))
}
