package sse

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/psyb0t/essessey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Conformance tests for the SSE wire format as specified by the WHATWG HTML
// Living Standard under "Server-sent events".
//
// Every case here corresponds to a rule the previous implementation broke, and
// each was confirmed against the real code before the fix: the framer emitted
// multi-line payloads as one `data:` line (so a payload containing a blank line
// forged a second event), and the parser matched the literal prefix `"data: "`
// pair-wise instead of running the format's state machine (so a conformant
// `data:x` produced nothing, and a second `data:` line was silently dropped).

// collect drains a Source into a slice so a test can assert on how many events
// a stream produced, not just on the first one. Event COUNT is the assertion
// that catches both halves of the old framing bug — a forged extra event and a
// silently swallowed one.
func collect(t *testing.T, stream string) []essessey.Event {
	t.Helper()

	src := NewSource(strings.NewReader(stream))

	var events []essessey.Event

	for {
		ev, err := src.Next(context.Background())
		if err != nil {
			require.ErrorIs(t, err, essessey.ErrNoMoreEvents)

			break
		}

		events = append(events, ev)
	}

	return events
}

func TestFrameLines_SpecShape(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		ev   essessey.Event
		want string
	}{
		{
			name: "plain event",
			ev: essessey.Event{
				Event: "ping",
				Data:  json.RawMessage(`{"a":1}`),
			},
			want: "event: ping\ndata: {\"a\":1}\n\n",
		},
		{
			name: "id emitted before event",
			ev: essessey.Event{
				ID:    "42",
				Event: "ping",
				Data:  json.RawMessage(`{}`),
			},
			want: "id: 42\nevent: ping\ndata: {}\n\n",
		},
		{
			// An empty id must NOT be written: an empty `id:` field resets the
			// receiver's last-event-ID rather than being ignored, destroying
			// the resume point established by earlier events.
			name: "empty id omitted entirely",
			ev: essessey.Event{
				Event: "ping",
				Data:  json.RawMessage(`{}`),
			},
			want: "event: ping\ndata: {}\n\n",
		},
		{
			// The core framing rule: one data field PER LINE.
			name: "multi-line payload becomes one data field per line",
			ev: essessey.Event{
				Event: "chunk",
				Data:  json.RawMessage("{\n  \"a\": 1\n}"),
			},
			want: "event: chunk\ndata: {\ndata:   \"a\": 1\ndata: }\n\n",
		},
		{
			// The payload's blank line must not terminate the event.
			name: "payload containing a blank line stays one event",
			ev: essessey.Event{
				Event: "chunk",
				Data:  json.RawMessage("{\n\n}"),
			},
			//nolint:dupword // repeated data: fields ARE the wire format here
			want: "event: chunk\ndata: {\ndata: \ndata: }\n\n",
		},
		{
			name: "empty payload still writes one data field",
			ev: essessey.Event{
				Event: "chunk",
				Data:  json.RawMessage(""),
			},
			want: "event: chunk\ndata: \n\n",
		},
		{
			// A newline in an event type or id would end the line and let the
			// rest be read as further fields — a caller could forge events.
			name: "newlines stripped from event type",
			ev: essessey.Event{
				Event: "a\nevent: forged",
				Data:  json.RawMessage(`{}`),
			},
			want: "event: aevent: forged\ndata: {}\n\n",
		},
		{
			name: "newlines and NUL stripped from id",
			ev: essessey.Event{
				ID:    "1\n2\x003",
				Event: "ping",
				Data:  json.RawMessage(`{}`),
			},
			want: "id: 123\nevent: ping\ndata: {}\n\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, FrameLines(tc.ev))
		})
	}
}

func TestFrameLines_NeverForgesAnExtraEvent(t *testing.T) {
	t.Parallel()

	// The property that the old framer violated: whatever the payload
	// contains, ONE Event in must produce exactly ONE event terminator.
	testCases := []struct {
		name    string
		payload string
	}{
		{"blank line", "{\n\n}"},
		{"trailing newline", "{}\n"},
		{"leading newline", "\n{}"},
		{"crlf", "{\r\n}"},
		{"lone cr", "{\r}"},
		{"many blank lines", "a\n\n\n\nb"},
		{"looks like a frame", "x\n\nevent: forged\ndata: pwned\n\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			frame := FrameLines(essessey.Event{
				Event: "chunk",
				Data:  json.RawMessage(tc.payload),
			})

			events := collect(t, frame)
			require.Len(t, events, 1, "frame = %q", frame)
			assert.Equal(t, essessey.EventType("chunk"), events[0].Event)
		})
	}
}

func TestRoundTrip_PayloadSurvivesByteForByte(t *testing.T) {
	t.Parallel()

	// The failure that started this: the old pair sink+source could not
	// round-trip its own output. A payload of "{\n\"a\":1\n}" came back as
	// "{" — truncated at the first newline, remainder discarded, no error.
	testCases := []struct {
		name    string
		payload string
	}{
		{"compact json", `{"a":1}`},
		{"indented json", "{\n  \"a\": 1\n}"},
		{"blank line inside", "{\n\n}"},
		{"leading spaces preserved", "   indented"},
		{"unicode", `{"s":"héllo ✅"}`},
		{"empty", ""},
		{"single newline", "\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := essessey.Event{
				ID:    "id-1",
				Event: "chunk",
				Data:  json.RawMessage(tc.payload),
			}

			events := collect(t, FrameLines(original))

			// Note an empty payload DOES still produce an event. Writing
			// `data:` with an empty value appends a newline to the receiver's
			// data buffer, so the buffer is not empty at dispatch — it is only
			// an event that never had a data field at all that gets discarded.
			// This assertion started out backwards and the test caught it.
			require.Len(t, events, 1)
			assert.Equal(t, tc.payload, string(events[0].Data))
			assert.Equal(t, original.Event, events[0].Event)
			assert.Equal(t, original.ID, events[0].ID)
		})
	}
}

func TestSource_SpecParsing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		stream    string
		wantCount int
		wantData  string
		wantEvent essessey.EventType
	}{
		{
			// The space after the colon is OPTIONAL. Requiring it made every
			// conformant producer that omits it unreadable.
			name:      "no space after colon",
			stream:    "event:ping\ndata:hello\n\n",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "ping",
		},
		{
			// Exactly ONE leading space is removed, not all whitespace.
			name:      "only one space is stripped",
			stream:    "event: ping\ndata:  two spaces\n\n",
			wantCount: 1,
			wantData:  " two spaces",
			wantEvent: "ping",
		},
		{
			// Multiple data fields join with a newline into ONE event. The old
			// parser returned one event holding only the first line.
			name:      "multiple data fields join",
			stream:    "event: ping\ndata: line1\ndata: line2\n\n",
			wantCount: 1,
			wantData:  "line1\nline2",
			wantEvent: "ping",
		},
		{
			name:      "crlf terminators",
			stream:    "event: ping\r\ndata: hello\r\n\r\n",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "ping",
		},
		{
			name:      "lone cr terminators",
			stream:    "event: ping\rdata: hello\r\r",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "ping",
		},
		{
			name:      "leading BOM is stripped",
			stream:    "\ufeffevent: ping\ndata: hello\n\n",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "ping",
		},
		{
			name:      "comments ignored",
			stream:    ": keep-alive\nevent: ping\n: mid\ndata: hello\n\n",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "ping",
		},
		{
			// An event with no data field is discarded, not delivered empty.
			name:      "event with no data is discarded",
			stream:    "event: ping\n\nevent: ping\ndata: real\n\n",
			wantCount: 1,
			wantData:  "real",
			wantEvent: "ping",
		},
		{
			// A data field with an empty value is still a data field, so the
			// event exists and carries an empty payload.
			name:      "empty data field still dispatches",
			stream:    "event: ping\ndata:\n\n",
			wantCount: 1,
			wantData:  "",
			wantEvent: "ping",
		},
		{
			name:      "unknown fields ignored",
			stream:    "event: ping\nbogus: x\ndata: hello\n\n",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "ping",
		},
		{
			// A line with no colon is a field name with an empty value; an
			// unknown one is simply ignored rather than derailing the event.
			name:      "colonless line ignored as unknown field",
			stream:    "event: ping\nbogus\ndata: hello\n\n",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "ping",
		},
		{
			// Unterminated at EOF: the producer never closed the event, so the
			// payload may be partial and is discarded rather than delivered.
			name:      "unterminated event at EOF is dropped",
			stream:    "event: ping\ndata: hello\n",
			wantCount: 0,
		},
		{
			name:      "data with no event type still dispatches",
			stream:    "data: hello\n\n",
			wantCount: 1,
			wantData:  "hello",
			wantEvent: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events := collect(t, tc.stream)
			require.Len(t, events, tc.wantCount)

			if tc.wantCount == 0 {
				return
			}

			assert.Equal(t, tc.wantData, string(events[0].Data))
			assert.Equal(t, tc.wantEvent, events[0].Event)
		})
	}
}

func TestSource_LastEventID(t *testing.T) {
	t.Parallel()

	t.Run("id persists to later events", func(t *testing.T) {
		t.Parallel()

		// The id describes the stream position, so it carries forward to
		// events that do not restate it — that is what lets a client resume
		// from the last id it actually saw.
		events := collect(t,
			"id: 1\nevent: a\ndata: x\n\n"+
				"event: b\ndata: y\n\n"+
				"id: 3\nevent: c\ndata: z\n\n")

		require.Len(t, events, 3)
		assert.Equal(t, "1", events[0].ID)
		assert.Equal(t, "1", events[1].ID, "id must persist across events")
		assert.Equal(t, "3", events[2].ID)
	})

	t.Run("empty id resets the resume point", func(t *testing.T) {
		t.Parallel()

		events := collect(t,
			"id: 1\nevent: a\ndata: x\n\n"+
				"id:\nevent: b\ndata: y\n\n")

		require.Len(t, events, 2)
		assert.Equal(t, "1", events[0].ID)
		assert.Empty(t, events[1].ID)
	})

	t.Run("id with NUL ignored, previous kept", func(t *testing.T) {
		t.Parallel()

		events := collect(t,
			"id: 1\nevent: a\ndata: x\n\n"+
				"id: 2\x003\nevent: b\ndata: y\n\n")

		require.Len(t, events, 2)
		assert.Equal(t, "1", events[0].ID)
		assert.Equal(t, "1", events[1].ID,
			"a malformed id must not destroy a valid resume point")
	})

	t.Run("LastEventID accessor tracks the stream", func(t *testing.T) {
		t.Parallel()

		src := NewSource(strings.NewReader("id: 7\nevent: a\ndata: x\n\n"))

		_, err := src.Next(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "7", src.LastEventID())
	})
}

func TestSource_Retry(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		stream string
		want   time.Duration
	}{
		{
			"numeric retry accepted",
			"retry: 2500\ndata: x\n\n",
			2500 * time.Millisecond,
		},
		{"non-numeric ignored", "retry: soon\ndata: x\n\n", 0},
		{"negative ignored", "retry: -5\ndata: x\n\n", 0},
		{"empty ignored", "retry:\ndata: x\n\n", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := NewSource(strings.NewReader(tc.stream))

			_, err := src.Next(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tc.want, src.Retry())
		})
	}

	t.Run("retry alone produces no event", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, collect(t, "retry: 1000\n\n"))
	})
}

func TestFrameRetry(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"whole seconds", 3 * time.Second, "retry: 3000\n\n"},
		{"milliseconds", 250 * time.Millisecond, "retry: 250\n\n"},
		{"negative clamps to zero", -time.Second, "retry: 0\n\n"},
		{"sub-millisecond truncates to zero", time.Microsecond, "retry: 0\n\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, FrameRetry(tc.d))
		})
	}
}

func TestFrameComment(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ": keep-alive\n", FrameComment("keep-alive"))

	// A comment must not be able to break out of its own line.
	assert.Equal(t,
		": aevent: forged\n",
		FrameComment("a\nevent: forged"),
	)

	// And a receiver must skip it without producing an event.
	assert.Empty(t, collect(t, FrameComment("keep-alive")))
}
