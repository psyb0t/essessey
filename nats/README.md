# nats — publish events to NATS subjects

A `Sink` that publishes `essessey.Event` to NATS, and a `Source` that turns a
subscription callback back into an `essessey.Source`.

NATS already delivers discrete messages, so nothing here frames anything — that
is the `sse` package's job and only its job.

## Contents

- [Publishing](#publishing)
- [The subject scheme](#the-subject-scheme)
- [Subscribing](#subscribing)
- [The envelope is not identical to other bindings](#the-envelope-is-not-identical-to-other-bindings)
- [Ordering and delivery caveats](#ordering-and-delivery-caveats)

## Publishing

This package never imports a NATS client. It asks for one method:

```go
type Publisher interface {
	Publish(subject string, data []byte) error
}
```

`*nats.Conn` satisfies it as-is, so you pass your own connection and this
package stays dependency-free:

```go
conn, err := nats.Connect(nats.DefaultURL)
if err != nil {
	return err
}

sink := essnats.NewSink(conn, "chat.conv_1")
pub := essessey.NewPublisher(ctx, sink)

_ = pub.SendStreamPreamble("msg_1", "conv_1", "some-model")
```

`Emit` publishes `ev.Data` **raw** — no framing, no envelope. A `*nats.Conn` is
safe for concurrent publishes, so the sink needs no lock of its own.

## The subject scheme

The subject is `subjectPrefix` + `.` + the event type:

```
chat.conv_1.message_start
chat.conv_1.content_block_delta
chat.conv_1.message_stop
```

An empty prefix publishes to the bare event type. Putting the type in the
subject is what lets a subscriber filter with a wildcard instead of decoding
every payload to find out whether it cares:

```go
conn.Subscribe("chat.conv_1.*", handler)              // one conversation
conn.Subscribe("chat.*.content_block_delta", handler) // deltas everywhere
```

## Subscribing

`Source` is a queue with an `essessey.Source` face. Wire `Deliver` into your
subscription callback and pull events off with `Next`:

```go
src := essnats.NewSource()

sub, err := conn.Subscribe("chat.conv_1.*", func(msg *nats.Msg) {
	src.Deliver(essessey.Event{
		Event: eventTypeFromSubject(msg.Subject),
		Data:  msg.Data,
	})
})

defer func() {
	_ = sub.Unsubscribe()
	src.Close()
}()

parsed := essessey.Reassemble(ctx, src)
fmt.Println(parsed.Text)
```

`Close` stops the source; `Next` drains whatever is already buffered and then
returns `essessey.ErrNoMoreEvents`. Delivering after `Close` is a no-op that
warns rather than panicking, because a subscription callback can still be
in flight when you tear down.

## The envelope is not identical to other bindings

This is the part that surprises people, so it is stated plainly rather than
implied away.

| binding | how the client receives the envelope |
|---|---|
| **ws** | the whole `Event` as one JSON object — `id`, `event`, `data` inline |
| **sse** | `id:`, `event:` and `data:` as wire fields |
| **nats** | payload raw; the event type is in the SUBJECT; **the id is not carried at all** |

The `Data` payload is byte-identical everywhere. The envelope is not, and a NATS
subscriber has the most reconstruction to do — you rebuild the event type from
the subject you matched, as `eventTypeFromSubject` does above.

`Event.ID` has nowhere to go here. Core NATS messages carry the payload and the
subject; there is no envelope field for it. If you need ids on this binding,
carry them in a message header yourself, or use a binding that has room for
them. Resume-from-`Last-Event-ID` is an SSE concept anyway — NATS has no
equivalent reconnect semantics.

## Ordering and delivery caveats

Worth knowing before streaming tokens over this:

- **The subject-per-event-type scheme fans one ordered stream across N
  subjects.** Core NATS preserves order per publisher-subscriber pair on a
  connection, which usually holds — but it stops being a guarantee the moment
  there is a queue group, a cluster hop, or JetStream with multiple subjects.
  If strict order matters, publish to a single subject and put the type in the
  payload instead.
- **Core NATS is at-most-once with no redelivery.** A slow consumer is dropped
  silently. For a token stream that is a corrupted render with no error
  anywhere.

Neither is a bug in this package — they are properties of the transport you
chose — but they decide whether this binding suits your stream.
