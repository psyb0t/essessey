package elelemstream

import (
	"testing"

	"github.com/psyb0t/elelem"
	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
)

func TestMapStopReason(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		finishReason elelem.FinishReason
		hasToolCalls bool
		want         essessey.StopReason
	}{
		{
			name:         "length truncation wins over tool calls",
			finishReason: elelem.FinishReasonLength,
			hasToolCalls: true,
			want:         essessey.StopReasonMaxTokens,
		},
		{
			name:         "context exceeded truncation, no tool calls",
			finishReason: elelem.FinishReasonContextExceeded,
			hasToolCalls: false,
			want:         essessey.StopReasonMaxTokens,
		},
		{
			name:         "tool calls, clean finish",
			finishReason: elelem.FinishReasonToolCalls,
			hasToolCalls: true,
			want:         essessey.StopReasonToolUse,
		},
		{
			name:         "clean finish, no tool calls",
			finishReason: elelem.FinishReasonStop,
			hasToolCalls: false,
			want:         essessey.StopReasonEndTurn,
		},
		{
			name:         "unset finish reason, no tool calls",
			finishReason: elelem.FinishReasonUnset,
			hasToolCalls: false,
			want:         essessey.StopReasonEndTurn,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := MapStopReason(tc.finishReason, tc.hasToolCalls)
			assert.Equal(t, tc.want, got)
		})
	}
}
