---
title: "Resuming a dropped stream"
description: "Let an essessey client reconnect mid-turn: stamping event ids, MultiSink fan-out, the EventStore interface, InMemoryEventStore, and replay rules."
nav_order: 7
---

# Resuming a dropped stream

`Event.ID` gives a client a resume point; `EventStore` is what lets a server honour it. The protocol gives you the mechanism, not the retention: you can only resume to an event you still have. The code is in [`store.go`](https://github.com/psyb0t/essessey/blob/main/store.go) and [`multisink.go`](https://github.com/psyb0t/essessey/blob/main/multisink.go).

Resume-from-`Last-Event-ID` is an SSE idea. The [WebSocket binding](websocket.md) carries ids too, so a client of your own can use the same scheme there. The [NATS binding](nats.md) drops ids entirely.

## Giving events ids

The `Publisher` emits every event with an empty `ID`, and an event with no id cannot be resumed to. Stamp ids in a small `Sink` that sits in front of everything else:

```go
type idSink struct {
	mu   sync.Mutex
	next essessey.Sink
	seq  uint64
}

func (s *idSink) Emit(ctx context.Context, ev essessey.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	ev.ID = strconv.FormatUint(s.seq, 10)

	return s.next.Emit(ctx, ev)
}
```

Holding the lock across `next.Emit` keeps ids in the same order as the events on the wire. Ids must be unique within a stream: `InMemoryEventStore` moves the resume point to the later of two events that share an id, so resuming from it skips everything in between.

## MultiSink

`MultiSink` fans one `Emit` out to several sinks, in order. It exists because every other sink is terminal; retention is the motivating case, and it also covers a wire sink plus an audit log, or SSE plus NATS.

```go
live := essessey.NewMultiSink(httpSink, store.SinkFor(streamID))
```

- It does not stop at the first failure. A full store or a broken audit sink must not cut off the client's stream. Every failure comes back joined into one error, so a non-nil error means at least one destination missed the event, not that it went nowhere.
- Zero sinks is allowed and discards everything, so you can build the list conditionally.
- It holds no state of its own; it is safe for concurrent use when the sinks it wraps are.

## Capture and replay

Capture composes rather than wrapping. `store.SinkFor(streamID)` is the store's write half, so retention is just another destination:

```go
store, err := essessey.NewInMemoryEventStore(256) // per stream, oldest evicted
if err != nil {
	return err
}

stamped := &idSink{next: essessey.NewMultiSink(httpSink, store.SinkFor(streamID))}
pub := essessey.NewPublisher(ctx, stamped)
```

A capacity of zero or less returns `ErrInvalidCapacity` rather than a store that accepts every append and resumes nothing.

On reconnect, ask what came after the client's last id, hand it to a `SliceSource`, and pump it through the ordinary wire sink. It is the same path a live stream uses, so the two cannot drift and the client cannot tell them apart:

```go
lastEventID := r.Header.Get("Last-Event-ID")

events, known, err := store.Since(ctx, streamID, lastEventID)
if err != nil {
	return err
}

if !known {
	// Not retained: evicted, never seen, or from a previous process.
	// Restart the stream. Do NOT guess.
}

src := essessey.NewSliceSource(events)
for {
	ev, err := src.Next(ctx)
	if errors.Is(err, essessey.ErrNoMoreEvents) {
		break
	}

	if err != nil {
		return err
	}

	if err := httpSink.Emit(ctx, ev); err != nil {
		return err
	}
}
```

That `known` flag is the whole design. When a resume point is missing, replaying from the start duplicates everything the client already rendered, and replaying from now silently drops the events in between, the exact gap ids exist to prevent. Only the caller knows which is acceptable, so the store refuses to choose. `known` true with an empty slice means the client is up to date.

## Know this before wiring it up

- **Replay through the wire sink alone, never the `MultiSink`.** Otherwise each reconnect re-appends what it is replaying and the store grows without bound.
- **`streamID` is a security boundary.** On reconnect it comes from the client, and `Since` only knows map keys, not owners. Replay keyed only by event id would let a client that presents an id receive someone else's events. Authorize the resume exactly as you authorize opening the stream.
- **Events without an id are still retained and replayed**, they just cannot be resumed to. Dropping them would silently skip real content.
- **An id-less frame still moves an SSE client's resume point.** The format sets the last event id before it discards an event with no data, so a bare `id:` frame advances the client without delivering anything.

## EventStore

```go
type EventStore interface {
	Append(ctx context.Context, streamID string, ev Event) error
	Since(ctx context.Context, streamID string, lastEventID string) ([]Event, bool, error)
	Clear(ctx context.Context, streamID string) error
}
```

`streamID` is an opaque key. Using the id the client already learned from `message_start` is the common case, but keying retention at a finer grain (per turn, per connection) is a legitimate choice that trades a smaller buffer for losing replay of earlier turns.

`InMemoryEventStore` is per process and bounded, a reasonable default and a reference implementation:

- Each stream gets a ring buffer of `capacity` events; the oldest is evicted first.
- An index from event id to position makes `Since` a map lookup rather than a scan.
- `SinkFor(streamID)` returns a `Sink` that appends to that stream.
- `Clear(streamID)` drops a finished stream. Nothing expires on its own, so call it when a stream is done or memory grows with every stream you ever served.
- It is safe for concurrent use.

Anything that must survive a restart or span replicas wants its own `EventStore` against shared storage.
