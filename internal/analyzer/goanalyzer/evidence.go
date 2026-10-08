package goanalyzer

import (
	"bytes"
	"go/ast"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gabipuzon/nami/internal/graph"
)

func sourceEvidence(file sourceFile, node ast.Node, edge graph.Edge) graph.SourceEvidence {
	start, end := file.fset.PositionFor(node.Pos(), false), file.fset.PositionFor(node.End(), false)
	content := file.content[start.Offset:end.Offset]
	truncated := len(content) > 8192
	if truncated {
		content = content[:8192]
		for !utf8.Valid(content) && len(content) > 0 {
			content = content[:len(content)-1]
		}
	}
	return graph.SourceEvidence{Edge: edge, Path: file.path, Line: start.Line, Column: sourceColumn(file.content, start.Offset), EndLine: end.Line, EndColumn: sourceColumn(file.content, end.Offset), Snippet: string(content), Hash: file.hash, Truncated: truncated}
}

// Editor columns count UTF-16 units; Go token offsets count UTF-8 bytes.
func sourceColumn(content []byte, offset int) int {
	start := bytes.LastIndexByte(content[:offset], '\n') + 1
	column := 1
	for _, r := range string(content[start:offset]) {
		column += utf16.RuneLen(r)
	}
	return column
}
