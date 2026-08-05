// Package ws provides essessey.Sink and essessey.Source bindings for
// WebSocket connections.
//
// WebSocket is message-oriented: every write is already a discrete frame,
// so this package needs no SSE-style framing — that framing exists only
// because an HTTP body is an undelimited byte stream. Sink sends the
// whole essessey.Event as one JSON message; Source hands events back
// exactly as they were delivered.
//
// This package deliberately does NOT import a websocket library such as
// github.com/gorilla/websocket. Conn declares the one method
// (WriteJSON(v any) error) this package actually needs, and gorilla's
// *websocket.Conn already satisfies it structurally — the caller passes
// their own connection, and essessey adds zero transport dependency. This
// is a design choice, not a missing feature: pulling in a websocket
// library would tie essessey's module graph to one driver version for the
// sake of a single method.
package ws
