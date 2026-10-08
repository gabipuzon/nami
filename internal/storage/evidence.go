package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/gabipuzon/nami/internal/graph"
)

const evidenceTable = `CREATE TABLE source_evidence (
 scan_id TEXT NOT NULL, ordinal INTEGER NOT NULL, fact TEXT NOT NULL,
 PRIMARY KEY(scan_id,ordinal), FOREIGN KEY(scan_id) REFERENCES snapshots(id)
)`

func saveEvidence(tx *sql.Tx, id string, evidence []graph.SourceEvidence) error {
	for i, fact := range evidence {
		data, err := json.Marshal(fact)
		if err != nil {
			return fmt.Errorf("encode source evidence: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO source_evidence VALUES (?,?,?)`, id, i, string(data)); err != nil {
			return fmt.Errorf("save source evidence: %w", err)
		}
	}
	return nil
}
func loadEvidence(db *sql.DB, id string) ([]graph.SourceEvidence, error) {
	rows, err := db.Query(`SELECT fact FROM source_evidence WHERE scan_id=? ORDER BY ordinal`, id)
	if err != nil {
		return nil, fmt.Errorf("load source evidence: %w", err)
	}
	defer rows.Close()
	var evidence []graph.SourceEvidence
	for rows.Next() {
		var data string
		var fact graph.SourceEvidence
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &fact); err != nil {
			return nil, fmt.Errorf("decode saved source evidence: %w", err)
		}
		evidence = append(evidence, fact)
	}
	return evidence, rows.Err()
}
