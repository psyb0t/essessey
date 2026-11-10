# Testing

Prefix: TEST. Commands are in `AGENTS.md`; every target runs in the `Dockerfile.dev` image.

- **TEST1:** Tests use testify (`assert`, `require`) and sit beside the code. Most are white-box (same package); `multisink_test.go` and `store_test.go` are black-box tests in `package essessey_test` and use only the exported API.
- **TEST2:** Tests always run with `-race`. Code that holds a mutex or a channel needs a test that exercises it concurrently.
- **TEST3:** The coverage floor is 90% across all packages (`-coverpkg=./...`), enforced by `scripts/test-coverage.sh`. Raise coverage for the code you add rather than lowering `MIN_TEST_COVERAGE`.
- **TEST4:** A binding test proves the event sequence survives: emit into the sink, read back through the matching source or an `InMemorySink`/`SliceSource` pair, and compare. No test needs a real NATS server or WebSocket; fakes satisfy the one-method interfaces.
- **TEST5:** A change to block-index arithmetic in `elelemstream` updates `adapter_test.go`, which covers the single-call, parallel-call, non-zero `toolBase` and second-round cases.
- **TEST6:** `make sec` must stay clean. It fails on any govulncheck or semgrep finding; vendored code is excluded from semgrep.
