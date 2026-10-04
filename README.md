# Nami

Nami is a local-first codebase navigator. Phase 00 provides a working CLI and
development checks; repository analysis is not implemented yet.

## Local checks

Requires Go 1.25 or newer and `make`.

```sh
make check
make build
./bin/nami --help
```

`make check` verifies Go formatting, runs tests, and runs `go vet`. CI runs
`make check build` on pushes and pull requests.
