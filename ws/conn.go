package ws

// Conn is the minimal capability this package needs from a WebSocket
// connection: write a value as one JSON-encoded frame.
//
// gorilla's *websocket.Conn (github.com/gorilla/websocket) satisfies this
// method as-is. Callers pass their own connection — see the package doc
// comment for why this package never imports a websocket library itself.
type Conn interface {
	WriteJSON(v any) error
}
