# Testing

Prefix: TEST. Commands are in `AGENTS.md`; every target runs in the `Dockerfile.dev` image.

- **TEST1:** Most tests are white-box (same package); `multisink_test.go` and `store_test.go` are black-box tests in `package essessey_test` and use only the exported API.
- **TEST3:** The coverage floor is 90% across all packages (`-coverpkg=./...`), enforced by `scripts/test-coverage.sh`.
- **TEST4:** A binding test proves the event sequence survives: emit into the sink, read back through the matching source or an `InMemorySink`/`SliceSource` pair, and compare. No test needs a real NATS server or WebSocket; fakes satisfy the one-method interfaces.
- **TEST5:** A change to block-index arithmetic in `elelemstream` updates `adapter_test.go`, which covers the single-call, parallel-call, non-zero `toolBase` and second-round cases.
- **TEST6:** Vendored code is excluded from the semgrep run in `make sec`.
