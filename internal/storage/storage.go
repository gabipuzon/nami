package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	_ "modernc.org/sqlite"
)

const schemaVersion = 3

type Store struct {
	db      *sql.DB
	version int
}

type Snapshot struct {
	ID        string
	Root      string
	CreatedAt time.Time
	Status    string
	Result    analysis.Result
}

type Summary struct {
	ID        string
	Root      string
	CreatedAt time.Time
	Status    string
}

func Open(root string) (*Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository path: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("open repository store: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("open repository store: %q is not a directory", absRoot)
	}
	storeDir := filepath.Join(absRoot, ".nami")
	if err := os.MkdirAll(storeDir, 0700); err != nil {
		return nil, fmt.Errorf("create repository store: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(storeDir, "scans.db")), RawQuery: "_foreign_keys=1"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open repository store: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, version: schemaVersion}
	if err := store.initialize(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

// OpenReadOnly loads existing stores without creating directories, initializing
// tables, or migrating SQLite. Interfaces that promise no writes use this path.
func OpenReadOnly(root string) (*Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository path: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("open repository store: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("open repository store: %q is not a directory", absRoot)
	}
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(absRoot, ".nami", "scans.db")), RawQuery: "mode=ro&_foreign_keys=1"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only repository store: %w", err)
	}
	db.SetMaxOpenConns(1)
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("read store schema version: %w", err)
	}
	if version < 1 || version > schemaVersion {
		db.Close()
		return nil, fmt.Errorf("read-only store schema version %d is unsupported", version)
	}
	return &Store{db: db, version: version}, nil
}

func (s *Store) initialize() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read store schema version: %w", err)
	}
	if version < 0 || version > schemaVersion {
		return fmt.Errorf("unsupported store schema version %d (expected %d)", version, schemaVersion)
	}
	var foreignKeys int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		return fmt.Errorf("store foreign keys unavailable: %v", err)
	}
	if version == schemaVersion {
		return nil
	}
	if version == 1 || version == 2 {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("migrate store: %w", err)
		}
		defer tx.Rollback()
		statements := []string{}
		if version == 1 {
			statements = append(statements, `ALTER TABLE nodes ADD COLUMN import_count INTEGER`, `ALTER TABLE nodes ADD COLUMN export_count INTEGER`)
		}
		statements = append(statements, `ALTER TABLE nodes ADD COLUMN language TEXT`, `PRAGMA user_version = 3`)
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return fmt.Errorf("migrate store schema: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit store migration: %w", err)
		}
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("initialize store: %w", err)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE snapshots (
			id TEXT PRIMARY KEY, root TEXT NOT NULL, created_at TEXT NOT NULL, status TEXT NOT NULL,
			files_discovered INTEGER NOT NULL, supported_source_files INTEGER NOT NULL,
			files_analyzed INTEGER NOT NULL, files_skipped INTEGER NOT NULL, files_failed INTEGER NOT NULL,
			imports_discovered INTEGER NOT NULL, internal_resolved INTEGER NOT NULL,
			standard_library INTEGER NOT NULL, external INTEGER NOT NULL,
			unresolved INTEGER NOT NULL, cgo INTEGER NOT NULL, unclassified INTEGER NOT NULL
		)`,
		`CREATE TABLE nodes (
			scan_id TEXT NOT NULL, id TEXT NOT NULL, kind TEXT NOT NULL, path TEXT NOT NULL, name TEXT NOT NULL,
			import_count INTEGER, export_count INTEGER, language TEXT,
			PRIMARY KEY (scan_id, id), FOREIGN KEY (scan_id) REFERENCES snapshots(id)
		)`,
		`CREATE TABLE edges (
			scan_id TEXT NOT NULL, kind TEXT NOT NULL, from_id TEXT NOT NULL, to_id TEXT NOT NULL,
			PRIMARY KEY (scan_id, kind, from_id, to_id),
			FOREIGN KEY (scan_id, from_id) REFERENCES nodes(scan_id, id),
			FOREIGN KEY (scan_id, to_id) REFERENCES nodes(scan_id, id)
		)`,
		`CREATE TABLE issues (
			scan_id TEXT NOT NULL, ordinal INTEGER NOT NULL, kind TEXT NOT NULL,
			path TEXT NOT NULL, import_path TEXT NOT NULL, reason TEXT NOT NULL,
			PRIMARY KEY (scan_id, ordinal), FOREIGN KEY (scan_id) REFERENCES snapshots(id)
		)`,
		`PRAGMA user_version = 3`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("initialize store schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit store schema: %w", err)
	}
	return nil
}

func (s *Store) Save(root string, result analysis.Result) (Summary, error) {
	if result.Coverage.Status != "complete" && result.Coverage.Status != "completed_with_gaps" {
		return Summary{}, fmt.Errorf("cannot save incomplete scan status %q", result.Coverage.Status)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve repository path: %w", err)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Summary{}, fmt.Errorf("create scan ID: %w", err)
	}
	summary := Summary{ID: hex.EncodeToString(random[:]), Root: absRoot, CreatedAt: time.Now().UTC(), Status: result.Coverage.Status}
	tx, err := s.db.Begin()
	if err != nil {
		return Summary{}, fmt.Errorf("begin snapshot: %w", err)
	}
	defer tx.Rollback()
	c := result.Coverage
	_, err = tx.Exec(`INSERT INTO snapshots VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		summary.ID, summary.Root, summary.CreatedAt.Format(time.RFC3339Nano), summary.Status,
		c.FilesDiscovered, c.SupportedSourceFiles, c.FilesAnalyzed, c.FilesSkipped, c.FilesFailed,
		c.ImportsDiscovered, c.InternalResolved, c.StandardLibrary, c.External, c.Unresolved, c.Cgo, c.Unclassified)
	if err != nil {
		return Summary{}, fmt.Errorf("save snapshot metadata: %w", err)
	}
	for _, node := range result.Graph.Nodes {
		var imports, exports any
		if node.Kind == graph.File && node.HasImportCount {
			imports = node.ImportCount
		}
		if node.Kind == graph.File && node.HasExportCount {
			exports = node.ExportCount
		}
		if _, err := tx.Exec(`INSERT INTO nodes (scan_id,id,kind,path,name,import_count,export_count,language) VALUES (?,?,?,?,?,?,?,?)`, summary.ID, node.ID, node.Kind, node.Path, node.Name, imports, exports, node.Language); err != nil {
			return Summary{}, fmt.Errorf("save graph node %q: %w", node.ID, err)
		}
	}
	for _, edge := range result.Graph.Edges {
		if _, err := tx.Exec(`INSERT INTO edges VALUES (?,?,?,?)`, summary.ID, edge.Kind, edge.From, edge.To); err != nil {
			return Summary{}, fmt.Errorf("save graph edge %s %q -> %q: %w", edge.Kind, edge.From, edge.To, err)
		}
	}
	for i, issue := range result.Issues {
		if _, err := tx.Exec(`INSERT INTO issues VALUES (?,?,?,?,?,?)`, summary.ID, i, issue.Kind, issue.Path, issue.Import, issue.Reason); err != nil {
			return Summary{}, fmt.Errorf("save analysis issue: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, fmt.Errorf("commit snapshot: %w", err)
	}
	return summary, nil
}

func (s *Store) List() ([]Summary, error) {
	rows, err := s.db.Query(`SELECT id, root, created_at, status FROM snapshots ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()
	var scans []Summary
	for rows.Next() {
		var scan Summary
		var created string
		if err := rows.Scan(&scan.ID, &scan.Root, &created, &scan.Status); err != nil {
			return nil, fmt.Errorf("read snapshot summary: %w", err)
		}
		scan.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse snapshot creation time: %w", err)
		}
		scans = append(scans, scan)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	return scans, nil
}

func (s *Store) Load(id string) (Snapshot, error) {
	var snap Snapshot
	var created string
	c := &snap.Result.Coverage
	err := s.db.QueryRow(`SELECT id, root, created_at, status,
		files_discovered, supported_source_files, files_analyzed, files_skipped, files_failed,
		imports_discovered, internal_resolved, standard_library, external, unresolved, cgo, unclassified
		FROM snapshots WHERE id = ?`, id).Scan(
		&snap.ID, &snap.Root, &created, &snap.Status,
		&c.FilesDiscovered, &c.SupportedSourceFiles, &c.FilesAnalyzed, &c.FilesSkipped, &c.FilesFailed,
		&c.ImportsDiscovered, &c.InternalResolved, &c.StandardLibrary, &c.External, &c.Unresolved, &c.Cgo, &c.Unclassified)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, fmt.Errorf("snapshot %q not found", id)
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load snapshot: %w", err)
	}
	snap.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse snapshot creation time: %w", err)
	}
	c.Status = snap.Status
	counts := "import_count, export_count"
	if s.version == 1 {
		// Legacy snapshots lack source counts. Read them without migrating the
		// database or implying that missing counts are zero.
		counts = "NULL, NULL"
	}
	language := "language"
	if s.version < 3 {
		language = "NULL"
	}
	nodes, err := s.db.Query(`SELECT id, kind, path, name, `+language+`, `+counts+` FROM nodes WHERE scan_id = ? ORDER BY kind, id`, id)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load graph nodes: %w", err)
	}
	for nodes.Next() {
		var node graph.Node
		var imports, exports sql.NullInt64
		var language sql.NullString
		if err := nodes.Scan(&node.ID, &node.Kind, &node.Path, &node.Name, &language, &imports, &exports); err != nil {
			nodes.Close()
			return Snapshot{}, fmt.Errorf("read graph node: %w", err)
		}
		node.Language = language.String
		if imports.Valid {
			node.ImportCount, node.HasImportCount = int(imports.Int64), true
		}
		if exports.Valid {
			node.ExportCount, node.HasExportCount = int(exports.Int64), true
		}
		snap.Result.Graph.Nodes = append(snap.Result.Graph.Nodes, node)
	}
	err = nodes.Err()
	nodes.Close()
	if err != nil {
		return Snapshot{}, fmt.Errorf("load graph nodes: %w", err)
	}
	edges, err := s.db.Query(`SELECT kind, from_id, to_id FROM edges WHERE scan_id = ? ORDER BY kind, from_id, to_id`, id)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load graph edges: %w", err)
	}
	for edges.Next() {
		var edge graph.Edge
		if err := edges.Scan(&edge.Kind, &edge.From, &edge.To); err != nil {
			edges.Close()
			return Snapshot{}, fmt.Errorf("read graph edge: %w", err)
		}
		snap.Result.Graph.Edges = append(snap.Result.Graph.Edges, edge)
	}
	err = edges.Err()
	edges.Close()
	if err != nil {
		return Snapshot{}, fmt.Errorf("load graph edges: %w", err)
	}
	issues, err := s.db.Query(`SELECT kind, path, import_path, reason FROM issues WHERE scan_id = ? ORDER BY ordinal`, id)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load analysis issues: %w", err)
	}
	for issues.Next() {
		var issue analysis.Issue
		if err := issues.Scan(&issue.Kind, &issue.Path, &issue.Import, &issue.Reason); err != nil {
			issues.Close()
			return Snapshot{}, fmt.Errorf("read analysis issue: %w", err)
		}
		snap.Result.Issues = append(snap.Result.Issues, issue)
	}
	err = issues.Err()
	issues.Close()
	if err != nil {
		return Snapshot{}, fmt.Errorf("load analysis issues: %w", err)
	}
	return snap, nil
}
