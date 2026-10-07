# nami

nami is a local-first codebase navigator. It maps Go modules, packages, source
files, package-scope declarations, and proven internal imports from a local
repository.

```sh
go run ./cmd/nami map .
```

The output lists graph nodes, then `CONTAINS` and internal `IMPORTS`
edges. Imports whose internal targets cannot be established are listed as
`UNRESOLVED_IMPORT`. Invalid Go files and module metadata are reported too.
Other source languages are reported as unsupported. Standard-library and
external imports do not become graph nodes. nami does not analyze non-Go source.
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

## MCP

Coding agents can query a saved scan through a local stdio MCP server:

```sh
./bin/nami mcp /absolute/path/to/repository <scan-id>
```

Configure your MCP client with the absolute path to the built `nami` binary as
its command and `["mcp", "/absolute/path/to/repository", "<scan-id>"]` as its
arguments. The client launches the process and communicates over stdin/stdout;
diagnostics go to stderr.

The server exposes `nami_scan_info`, `nami_search_nodes`, `nami_inspect_node`,
`nami_package_dependencies`, `nami_package_dependents`, `nami_dependency_path`,
`nami_file_symbols`, and `nami_package_impact`. Search first, then inspect or
query the relevant node IDs. Package relationships include their saved import
evidence; paths follow only known `IMPORTS` edges in canonical or package scope.

MCP loads one saved snapshot and closes SQLite before handling requests. It
does not rescan, read current source, edit files, or change the database. Restart
it with another scan ID to query a newer snapshot. Check `nami_scan_info` for
analysis gaps; impact reports potential dependency reachability and preserves
the snapshot's incomplete status.

## Coverage

`COVERAGE status=complete` means nami found no analysis gaps. A successful map
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
