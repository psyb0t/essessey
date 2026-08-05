package elelemstream

import (
	"context"
	"testing"

	"github.com/psyb0t/elelem"
	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sentinels are only worth exporting if they survive the wrap — a caller
// matching with errors.Is must not have to know how many layers of context the
// adapter added on the way out.
func TestAdapter_SentinelsSurviveTheWrap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		call    func(*Adapter) error
		wantErr error
	}{
		{
			"delta before a round opened",
			func(a *Adapter) error {
				return a.onDelta(context.Background(), elelem.Delta{Text: "x"})
			},
			ErrRoundStreamNotInitialized,
		},
		{
			"assistant message before a round opened",
			func(a *Adapter) error {
				return a.onAssistantMessage(
					context.Background(), elelem.Message{},
				)
			},
			ErrRoundStreamNotInitialized,
		},
		{
			"tool result with nothing attached",
			func(a *Adapter) error {
				return a.onToolResult(
					context.Background(),
					elelem.ToolCallEvent{CallID: "call-1"},
				)
			},
			ErrToolResultMissing,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := essessey.NewInMemorySink()
			adapter := New(essessey.NewPublisher(t.Context(), sink))

			err := tc.call(adapter)

			require.ErrorIs(t, err, tc.wantErr)
			assert.Zero(t, sink.Len(),
				"a rejected callback must emit nothing")
		})
	}
}
