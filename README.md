# Nami

Nami is a local-first codebase navigator. It currently maps Go packages, Go
source files, and internal imports from a local repository.

```sh
go run ./cmd/nami map .
```

The output lists package and file nodes, then `CONTAINS` and internal `IMPORTS`
edges. Imports whose internal targets cannot be established are listed as
`UNRESOLVED_IMPORT`. Invalid Go files and module metadata are reported too.
Other source languages are reported as unsupported. Standard-library and
external imports do not become graph nodes. This phase does not analyze symbols
or non-Go source. Nami uses the local `go` command to read module paths; mapping
does not need a network connection.

## Local checks

Requires Go 1.25 or newer and `make`.

```sh
make check
make build
./bin/nami --help
```

`make check` verifies Go formatting, runs tests, and runs `go vet`. CI runs
`make check build` on pushes and pull requests.
