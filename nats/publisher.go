package nats

// Publisher is the minimal capability this package needs from a NATS
// client: publish raw bytes to a subject.
//
// *nats.Conn (github.com/nats-io/nats.go) satisfies this method as-is.
// Callers pass their own connection — see the package doc comment for why
// this package never imports the nats client library itself.
type Publisher interface {
	Publish(subject string, data []byte) error
}
