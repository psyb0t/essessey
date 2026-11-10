# Dependencies

Prefix: DEP. Enforced by the `depguard` rule in `.golangci.yml`, which allows only the standard library, `github.com/stretchr/testify/` and `github.com/psyb0t` modules.

- **DEP1:** The core package, `sse`, `nats` and `ws` import no transport SDK. A binding declares the smallest interface it needs (`nats.Publisher` with `Publish(subject string, data []byte) error`, `ws.Conn` with `WriteJSON(v any) error`) and the caller passes in their own client. Do not add `nats.go`, gorilla/websocket or any other client to `go.mod`.
- **DEP2:** `github.com/psyb0t/elelem` is imported only by `elelemstream/`. The core package and the transport bindings never import it, so a caller who does not use elelem never pulls it in.
- **DEP3:** Errors are wrapped with `github.com/psyb0t/ctxerrors` and logs go through `github.com/psyb0t/ctxscope`. Do not add another error or logging library.
- **DEP4:** After changing `go.mod`, run `make dep` (it runs `go mod tidy` and `go mod vendor`) and commit the `vendor/` changes with it. Never edit files under `vendor/` by hand.
