# nami

nami is a local-first codebase navigator. It maps Go and Python packages, source
files, supported declarations, and proven internal imports from a local
repository.

```sh
make build
./bin/nami map /path/to/repository
```

The binary scans, saves, serves the map on localhost, and opens your browser.
Press Ctrl-C to stop. Building requires Node.js 20.9+ and the existing frontend
dependencies (`cd web && npm ci`); running the binary needs no Node.js.
Use `map --no-open` to open the printed URL yourself, or `map --no-serve` for
terminating CLI output. `show` reprints a saved scan; `serve <directory> <scan-id>`
opens its map at `http://127.0.0.1:7331` without launching a browser.

Go analysis uses the local Go command; Python analysis uses Python 3's AST parser.
Unsupported languages and unresolved relationships are reported explicitly.
No network connection or AI provider is needed for analysis.

## Frontend development

Run `go run ./cmd/nami map --no-serve .`, then
`go run ./cmd/nami serve . <scan-id>`. In another terminal run `npm run dev`
inside `web`, and open `http://localhost:3000`. The development server proxies
API requests to the local Go server.

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
`nami_file_symbols`, `nami_package_impact`, and `nami_source_evidence`. Search first, then inspect or
query the relevant node IDs. Package relationships include their saved import
evidence; paths follow only known `IMPORTS` edges in canonical or package scope.

MCP loads one saved snapshot and closes SQLite before handling requests. It
does not rescan, read current source, edit files, or change the database. Restart
it with another scan ID to query a newer snapshot. Check `nami_scan_info` for
analysis gaps; impact reports potential dependency reachability and preserves
the snapshot's incomplete status.

## Saved evidence and refresh

Inspector shows saved import and supported export-use excerpts, with source
locations and [VS Code links](https://code.visualstudio.com/docs/configure/command-line#opening-vs-code-with-urls).
Current files are checked against saved hashes and labelled unchanged, changed,
missing, or unreadable. Editor links open current source; excerpts remain saved
facts. Older snapshots explicitly lack recorded source locations.

The map shows its scan time. **Rescan** reads the current repository and ignore
rules, saves a new snapshot, and starts at a fresh overview. The old map remains
available during scanning and on failure. CLI queries and MCP stay pinned to
their requested saved scan. Display settings persist across rescans.

Startup loads the package overview. Explorer/search/Inspector page saved detail;
canvas expansion loads files and declarations explicitly. Supporting relationship
facts remain available on old scans even when source snippets are unavailable.

## Scan exclusions

Put a `.namiignore` file at the repository root before running `nami map .`:

```gitignore
# Root directory and matching file names
/generated/
*_test.go

# Keep this file despite the earlier rule
!important_test.go

# Exclude directory contents while retaining one subdirectory
examples/*
!examples/keep/
```

Patterns use [Git-style matching](https://git-scm.com/docs/gitignore): `*`, `?`,
character classes, `**`, root anchors, directory-only trailing slashes, comments
and ordered `!` exceptions. A basename pattern matches at any depth; a pattern
with a leading or middle slash matches from the scanned root. An exception
cannot reopen a file inside a pruned directory: include its parent first.
Escape a leading `#` or `!` and literal wildcard characters with a backslash.

Only the root `.namiignore` is read. Git ignore files and nested `.namiignore`
files do not control the scan. Rules apply to tracked and untracked files alike.
The existing built-in directory exclusions still apply and cannot be reopened.
The root control file itself is outside the scanned file count. Unreadable,
nonregular or malformed configuration fails the scan with a clear error.

Each matching file or pruned directory is reported with its matching rule and
line, separately from analysis issues. A directory counts once; its contents
are not enumerated. Intentional exclusions do not count as skipped or failed
files. Complete coverage describes the included scan scope. Imports whose
targets are unavailable still report analysis gaps, without invented edges.
Go package metadata lookup omits explicitly excluded Go files too.

Exclusions are saved with the snapshot and exposed by the CLI, API, MCP and Web
coverage panel. Focus and impact report the excluded path count separately from
analysis gaps. Changing rules requires a new scan; reopening an old scan keeps
its saved exclusions and graph.

## Coverage

`COVERAGE status=complete` means nami found no analysis gaps within the included scan scope. A successful map
with skipped or failed files, unresolved imports, or module errors reports
`completed_with_gaps` and lists each reason.

File counts cover entries in scanned directories; ignored directories are
excluded. `supported_source_files` counts Go and Python candidates, including Go files
skipped because they are not regular files. `files_analyzed` counts Go and Python files
parsed successfully. `files_failed` counts Go and Python files that could not be read or
parsed. `files_skipped` counts nonregular entries and unsupported source files.

Import counts cover import declarations in successfully parsed source files.
Internal imports are counted when resolved, even if repeated declarations
produce one graph edge. Standard-library imports come from Go's package list;
external imports require an unreplaced, unexcluded dependency declared in
`go.mod` with no active Go workspace.
`cgo_imports` counts the special `C` import. Imports without enough evidence
are `unclassified_imports` and include a reason.

## Local checks

Requires Go 1.25 or newer, Node.js 20.9 or newer, installed frontend dependencies, and `make`.

```sh
make check
make build
./bin/nami --help
```

`make check` verifies Go formatting, runs tests, and runs `go vet`. CI also runs
the web tests, lint, and build on pushes and pull requests.
