package graph

import (
	"crypto/sha256"
	"encoding/hex"
)

// SourceEvidence belongs to an occurrence, not to graph edge identity.
// Spans are one-based with exclusive ends and UTF-16 editor columns.
type SourceEvidence struct {
	Edge      Edge   `json:"edge"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line"`
	EndColumn int    `json:"end_column"`
	Snippet   string `json:"snippet"`
	Truncated bool   `json:"truncated"`
	Hash      string `json:"hash"`
}

func SourceHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}
