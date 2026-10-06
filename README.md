# Nami

Nami is a local-first codebase navigator. It maps Go modules, packages, source
files, package-scope declarations, and proven internal imports from a local
repository.

```sh
go run ./cmd/nami map .
```

The output lists graph nodes, then `CONTAINS` and internal `IMPORTS`
edges. Imports whose internal targets cannot be established are listed as
`UNRESOLVED_IMPORT`. Invalid Go files and module metadata are reported too.
Other source languages are reported as unsupported. Standard-library and
external imports do not become graph nodes. Nami does not analyze non-Go source.
It uses the local `go` command to read module paths; mapping
does not need a network connection.

## Web map

Requires Node.js 20.9 or newer. First save a scan and use the ID printed by
`map`:

```sh
go run ./cmd/nami map .
go run ./cmd/nami serve . <scan-id>
```

In another terminal:

```sh
cd web
npm install
npm run dev
```

Open `http://localhost:3000`. The browser uses the local API through a Next.js
rewrite. The server keeps showing the selected saved scan until restarted with
another ID.

## Coverage

`COVERAGE status=complete` means Nami found no analysis gaps. A successful map
with skipped or failed files, unresolved imports, or module errors reports
`completed_with_gaps` and lists each reason.

File counts cover entries in scanned directories; ignored directories are
excluded. `supported_source_files` counts Go candidates, including Go files
skipped because they are not regular files. `files_analyzed` counts Go files
parsed successfully. `files_failed` counts Go files that could not be read or
parsed. `files_skipped` counts nonregular entries and unsupported source files.

Import counts cover import declarations in successfully parsed Go files.
Internal imports are counted when resolved, even if repeated declarations
produce one graph edge. Standard-library imports come from Go's package list;
external imports require an unreplaced, unexcluded dependency declared in
`go.mod` with no active Go workspace.
`cgo_imports` counts the special `C` import. Imports without enough evidence
are `unclassified_imports` and include a reason.

## Local checks

Requires Go 1.25 or newer and `make`.

```sh
make check
make build
./bin/nami --help
```

`make check` verifies Go formatting, runs tests, and runs `go vet`. CI also runs
the web tests, lint, and build on pushes and pull requests.
