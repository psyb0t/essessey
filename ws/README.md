# ws

The WebSocket binding for essessey: a `Sink` that writes each `Event` as one `WriteJSON` frame, and a `Source` whose `Deliver` method you wire into a read loop. It never imports a WebSocket library; gorilla's `*websocket.Conn` already satisfies the one-method `Conn` interface.

gorilla permits one concurrent writer. The sink locks its own writes, but anything else writing to the same connection needs your own lock.

Full documentation: **[WebSocket binding](https://psyb0t.github.io/essessey/websocket/)**.
