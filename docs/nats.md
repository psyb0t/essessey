---
title: "NATS binding"
description: "The essessey nats package: publish events raw to per-event-type subjects, rebuild them from a subscription, and the ordering and id caveats."
parent: "Events and bindings"
nav_order: 2
---

# NATS binding

`github.com/psyb0t/essessey/nats` has a `Sink` that publishes events to NATS and a `Source` that turns a subscription callback back into an `essessey.Source`. The code is in [`nats/`](https://github.com/psyb0t/essessey/tree/main/nats).

NATS already delivers discrete messages, so nothing here frames anything. That's the [SSE binding](sse.md)'s job and only its job.

## Publishing

This package never imports a NATS client. It asks for one method:

```go
type Publisher interface {
	Publish(subject string, data []byte) error
}
```

`*nats.Conn` from `github.com/nats-io/nats.go` satisfies it as it is, so you pass your own connection:

```go
import (
	natsgo "github.com/nats-io/nats.go"

	"github.com/psyb0t/essessey"
	essnats "github.com/psyb0t/essessey/nats"
)

conn, err := natsgo.Connect(natsgo.DefaultURL)
if err != nil {
	return err
}

sink := essnats.NewSink(conn, "chat.conv_1")
pub := essessey.NewPublisher(ctx, sink)

if err := pub.SendStreamPreamble("msg_1", "conv_1", "some-model"); err != nil {
	return err
}
```

`Emit` publishes `ev.Data` **raw**: no framing, no envelope. A `*nats.Conn` is safe for concurrent publishes, so the sink has no lock of its own. If your `Publisher` is something else, it has to be safe for concurrent calls too.

## The subject scheme

The subject is `subjectPrefix` + `.` + the event type:

```text
chat.conv_1.message_start
chat.conv_1.content_block_delta
chat.conv_1.message_stop
```

An empty prefix publishes to the bare event type. Putting the type in the subject lets a subscriber filter with a wildcard instead of decoding every payload to find out whether it cares:

```go
conn.Subscribe("chat.conv_1.*", handler)              // one stream
conn.Subscribe("chat.*.content_block_delta", handler) // deltas everywhere
```

## Subscribing

`Source` is a queue with an `essessey.Source` face. Wire `Deliver` into your subscription callback and pull events off with `Next`:

```go
src := essnats.NewSource()

sub, err := conn.Subscribe("chat.conv_1.*", func(msg *natsgo.Msg) {
	src.Deliver(essessey.Event{
		Event: eventTypeFromSubject(msg.Subject),
		Data:  msg.Data,
	})
})
if err != nil {
	return err
}

defer func() {
	_ = sub.Unsubscribe()
	src.Close()
}()

parsed := essessey.Reassemble(ctx, src)
fmt.Println(parsed.Text)
```

`eventTypeFromSubject` is yours to write; with the scheme above it is the part after the last `.`.

- `Next` blocks until an event arrives, the source is closed and drained, or the context is done (that returns the wrapped context error).
- `Close` stops the source. `Next` drains whatever is already buffered and then returns `essessey.ErrNoMoreEvents`. `Close` can be called more than once.
- `Deliver` after `Close` is a no-op that logs a warning instead of panicking, because a subscription callback can still be in flight when you tear down.
- The queue holds 64 events. When it is full, `Deliver` blocks its caller (the NATS callback goroutine) until `Next` takes one.

`Reassemble` returns at `message_stop`, so in the example above the source is closed by the deferred function after the turn is complete. A producer that dies before `message_stop` leaves `Next` waiting; give `ctx` a deadline.

## The envelope

This is the part that surprises people, so it is stated plainly:

| Binding | How the client receives the envelope |
| --- | --- |
| WebSocket | the whole `Event` as one JSON object: `id`, `event`, `data` inline |
| SSE | `id:`, `event:` and `data:` as wire fields |
| NATS | payload raw; the event type is in the subject; **the id is not carried at all** |

The `Data` payload is byte-identical everywhere. The envelope is not, and a NATS subscriber has the most rebuilding to do: you get the event type back from the subject you matched. Every payload also repeats the event name in its own `type` field (see [Event protocol](protocol.md)).

`Event.ID` has nowhere to go here. Core NATS messages carry the payload and the subject, with no envelope field for it. If you need ids on this binding, carry them in a message header yourself, or use a binding that has room for them. Resume from `Last-Event-ID` is an SSE idea anyway; NATS has no matching reconnect behavior.

## Ordering and delivery caveats

Worth knowing before streaming tokens over this:

- **One subject per event type fans one ordered stream across several subjects.** Core NATS keeps order per publisher and subscriber pair on a connection, which usually holds, but it stops being a guarantee the moment there is a queue group, a cluster hop, or JetStream with several subjects. If strict order matters, publish to a single subject and put the type in the payload instead.
- **Core NATS is at-most-once with no redelivery.** A slow consumer is dropped silently. For a token stream that is a corrupted render with no error anywhere.

Neither is a bug in this package; they are properties of the transport you picked. They decide whether this binding suits your stream.
