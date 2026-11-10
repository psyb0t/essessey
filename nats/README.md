# nats

The NATS binding for essessey: a `Sink` that publishes `Event.Data` raw to `subjectPrefix.<event type>`, and a `Source` whose `Deliver` method you wire in as a subscription callback. It never imports a NATS client; `*nats.Conn` already satisfies the one-method `Publisher` interface.

The event id is not carried on this binding.

Full documentation, including the subject scheme and the ordering caveats: **[NATS binding](https://psyb0t.github.io/essessey/nats/)**.
