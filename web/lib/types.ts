export const declarationKinds = [
  "FUNCTION", "CLASS", "METHOD", "STRUCT", "INTERFACE", "TYPE", "VARIABLE", "CONSTANT",
] as const;

export type DeclarationKind = typeof declarationKinds[number];
export type NodeKind = "MODULE" | "PACKAGE" | "FILE" | DeclarationKind;
export type EdgeKind = "CONTAINS" | "IMPORTS" | "USES_EXPORT";

export interface GraphNode {
  id: string;
  kind: NodeKind;
  path: string;
  name: string;
  language?: string;
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
  exclusions?: { path: string; reason: string }[];
}

export interface Evidence {
  edge: GraphEdge;
  sources: GraphEdge[];
}

export interface PackageProjection {
  relationship_totals?: Record<string, number>;
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
  excluded_paths?: number;
}

export interface ApiError {
  error: {
    code: string;
    message: string;
  };
}

export interface LoadedData {
  childCounts?: Record<string, number>;
  fileCounts?: Record<string, number>;
  counts?: { packages: number; files: number; declarations: number };
  scan: Scan;
  canonicalGraph: Graph;
  packageProjection: PackageProjection;
}

export interface NeighborPage { nodes: GraphNode[]; total: number; offset: number }
export interface Neighborhood {
  context_graph?: Graph;
  focal: GraphNode;
  scope: "canonical" | "package";
  graph: Graph;
  dependencies: NeighborPage;
  dependents: NeighborPage;
  evidence: Evidence[];
  page_size: number;
  snapshot_id: string;
  coverage_status: Scan["status"];
  incomplete: boolean;
  excluded_paths?: number;
}

export interface SourceOccurrence {
  edge: GraphEdge;
  path: string;
  line: number;
  column: number;
  end_line: number;
  end_column: number;
  snippet: string;
  truncated: boolean;
  hash: string;
}
export interface SourceEvidencePage {
  items: SourceOccurrence[];
  total: number;
  offset: number;
  page_size: number;
  recorded: boolean;
}
export interface SourceStatus { status: "unchanged" | "changed" | "missing" | "unreadable" | "not_recorded"; path: string }

export interface GraphDetail {
  graph: Graph;
  package_projection: PackageProjection;
  child_counts: Record<string, number>;
  file_counts?: Record<string, number>;
}
export interface Overview extends GraphDetail { counts: { packages: number; files: number; declarations: number } }
export interface NodePage { nodes: GraphNode[]; total: number; offset: number }
export interface ChildrenPage extends GraphDetail, NodePage {}
export interface Inspection extends GraphDetail {
  node: GraphNode;
  parent: string;
  children: NodePage;
  dependencies: NodePage;
  dependents: NodePage;
}
export interface RelationshipFacts { items: GraphEdge[]; total: number; offset: number; graph: Graph }
