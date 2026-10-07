export const declarationKinds = [
  "FUNCTION", "METHOD", "STRUCT", "INTERFACE", "TYPE", "VARIABLE", "CONSTANT",
] as const;

export type DeclarationKind = typeof declarationKinds[number];
export type NodeKind = "MODULE" | "PACKAGE" | "FILE" | DeclarationKind;
export type EdgeKind = "CONTAINS" | "IMPORTS" | "USES_EXPORT";

export interface GraphNode {
  id: string;
  kind: NodeKind;
  path: string;
  name: string;
  import_count?: number;
  export_count?: number;
}

export interface GraphEdge {
  kind: EdgeKind;
  from: string;
  to: string;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface Coverage {
  files_discovered: number;
  supported_source_files: number;
  files_analyzed: number;
  files_skipped: number;
  files_failed: number;
  imports_discovered: number;
  internal_imports_resolved: number;
  standard_library_imports: number;
  external_imports: number;
  unresolved_imports: number;
  cgo_imports: number;
  unclassified_imports: number;
}

export interface Issue {
  kind: string;
  path: string;
  import: string;
  reason: string;
}

export interface Scan {
  id: string;
  root: string;
  created_at: string;
  status: "complete" | "completed_with_gaps";
  coverage: Coverage;
  issues: Issue[];
}

export interface Evidence {
  edge: GraphEdge;
  sources: GraphEdge[];
}

export interface PackageProjection {
  graph: Graph;
  evidence: Evidence[];
}

export interface Affected {
  id: string;
  distance: number;
}

export interface Impact {
  target: string;
  affected: Affected[];
  graph: Graph;
  coverage_status: Scan["status"];
  incomplete: boolean;
}

export interface ApiError {
  error: {
    code: string;
    message: string;
  };
}

export interface LoadedData {
  scan: Scan;
  canonicalGraph: Graph;
  packageProjection: PackageProjection;
}
