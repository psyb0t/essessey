// Package nats provides essessey.Sink and essessey.Source bindings for
// NATS.
//
// NATS is message-oriented: every publish is already a discrete message, so
// this package needs no SSE-style framing — that framing exists only
// because an HTTP body is an undelimited byte stream. Sink publishes
// essessey.Event.Data raw; Source hands events back exactly as they were
// delivered.
//
// This package deliberately does NOT import github.com/nats-io/nats.go.
// Publisher declares the one method (Publish(subject string, data []byte)
// error) this package actually needs, and *nats.Conn already satisfies it
// structurally — the caller passes their own client, and essessey adds
// zero transport dependency. This is a design choice, not a missing
// feature: pulling in the real client would tie essessey's module graph to
// one NATS driver version for the sake of a single method.
package nats
