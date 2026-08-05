// Package elelemstream bridges elelem's streaming callbacks to essessey's
// content-block protocol.
//
// An elelem round streams deltas, then (optionally) tool calls and their
// results, with no notion of "content block index" of its own. Adapter is the
// one place that arithmetic lives: get it wrong and a tool result renders
// into the wrong card client-side. Every caller wiring elelem to essessey
// binds one Adapter instead of reimplementing this.
package elelemstream

import (
	"context"

	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/elelem"
	"github.com/psyb0t/essessey"
)

// contentBlocksPerTool is how many content blocks one tool call produces on
// the wire: a tool_use block and its paired tool_result block.
const contentBlocksPerTool = 2

// Adapter binds elelem's streaming callbacks to an essessey Publisher,
// translating each round's deltas, tool calls, and tool results into
// correctly-indexed content blocks.
//
// Build one Adapter per in-flight elelem.Request — its fields track the
// current round's state and are not safe for concurrent Bind targets.
type Adapter struct {
	publisher *essessey.Publisher

	blockIndex    int
	rounds        int
	roundStream   *roundStream
	toolBase      int
	toolCallCount int
}

// New builds an Adapter that emits content blocks to pub.
func New(pub *essessey.Publisher) *Adapter {
	return &Adapter{publisher: pub}
}

// Rounds reports how many rounds have completed so far.
func (a *Adapter) Rounds() int {
	return a.rounds
}

// Bind registers the Adapter's callbacks on req and returns req, so callers
// can chain it into the rest of the elelem.Request build.
func (a *Adapter) Bind(req *elelem.Request) *elelem.Request {
	return req.
		OnRoundStart(a.onRoundStart).
		OnDelta(a.onDelta).
		OnAssistantMessage(a.onAssistantMessage).
		OnRoundEnd(a.onRoundEnd).
		OnToolCallStart(a.onToolCallStart).
		OnToolResult(a.onToolResult)
}

// onRoundStart opens a fresh roundStream at the block index the previous
// round (or the run's start) left off at. Nothing is emitted here — blocks
// open lazily, on first content.
func (a *Adapter) onRoundStart(
	_ context.Context,
	_ *elelem.RoundEvent,
) error {
	a.roundStream = newRoundStream(a.publisher, a.blockIndex)

	return nil
}

// onDelta forwards a streamed chunk to the round's thinking/text streamers.
func (a *Adapter) onDelta(ctx context.Context, delta elelem.Delta) error {
	if a.roundStream == nil {
		return ctxerrors.Wrap(ErrRoundStreamNotInitialized, "handle delta")
	}

	return a.roundStream.handleDelta(ctx, delta)
}

// onAssistantMessage closes out the round's thinking/text blocks and derives
// the tool block base: toolBase is wherever the round's content left off, and
// every tool_use/tool_result index for this round is computed relative to it.
func (a *Adapter) onAssistantMessage(
	ctx context.Context,
	message elelem.Message,
) error {
	if a.roundStream == nil {
		return ctxerrors.Wrap(
			ErrRoundStreamNotInitialized, "close assistant message",
		)
	}

	if err := a.roundStream.finish(ctx); err != nil {
		return err
	}

	a.blockIndex = a.roundStream.nextBlockIndex()
	a.toolBase = a.blockIndex
	a.toolCallCount = len(message.ToolCalls)

	return nil
}

// onRoundEnd tracks the completed round count.
func (a *Adapter) onRoundEnd(
	_ context.Context,
	event *elelem.RoundEvent,
) error {
	a.rounds = event.Round + 1

	return nil
}

// onToolCallStart emits the tool_use block for one call. Parallel calls in
// the same round each get their own slot: toolBase + the call's index.
func (a *Adapter) onToolCallStart(
	_ context.Context,
	event elelem.ToolCallEvent,
) error {
	if err := a.publisher.SendToolUseBlock(
		a.toolBase+event.Index,
		event.CallID,
		event.Name,
		string(event.Arguments),
	); err != nil {
		return ctxerrors.Wrap(err, "send tool_use block")
	}

	return nil
}

// onToolResult emits the tool_result block for one call, after every tool_use
// block in the round — hence the +toolCallCount offset — then, once the last
// result of the round has landed, advances blockIndex past all of them (2
// blocks per tool: one use, one result) so the next round starts clean.
func (a *Adapter) onToolResult(
	_ context.Context,
	event elelem.ToolCallEvent,
) error {
	if event.Result == nil {
		return ctxerrors.Wrapf(
			ErrToolResultMissing, "tool call %q", event.CallID,
		)
	}

	if err := a.publisher.SendToolResultBlock(
		a.toolBase+a.toolCallCount+event.Index,
		event.CallID,
		event.Result.Content,
		event.Result.IsError,
	); err != nil {
		return ctxerrors.Wrap(err, "send tool_result block")
	}

	if event.Index == a.toolCallCount-1 {
		a.blockIndex = a.toolBase + contentBlocksPerTool*a.toolCallCount
	}

	return nil
}
