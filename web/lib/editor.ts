// VS Code's file URL uses a path followed by one-based line and column.
export function vscodeLocation(path: string, line: number, column: number): string {
  const normalized=path.replaceAll("\\", "/");
  const encoded=normalized.split("/").map(segment=>encodeURIComponent(segment).replaceAll("%3A", ":")).join("/");
  return `vscode://file/${encoded.replace(/^\//, "")}:${line}:${column}`;
}
