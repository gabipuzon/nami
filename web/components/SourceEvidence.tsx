"use client";

import { useEffect, useState } from "react";
import { vscodeLocation } from "../lib/editor";
import { loadSourceEvidence, loadSourceStatus } from "../lib/api";
import type { GraphEdge, SourceEvidencePage, SourceOccurrence, SourceStatus } from "../lib/types";

function Occurrence({ item }: { item: SourceOccurrence }) {
  const [status, setStatus] = useState<SourceStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    loadSourceStatus(`file:${item.path}`, controller.signal).then(setStatus).catch((error: unknown) => {
      if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Unable to check current source.");
    });
    return () => controller.abort();
  }, [item.path, attempt]);
  const location = `${item.path}:${item.line}:${item.column}`;
  return <li className="source-occurrence">
    <code>{location}</code>
    <button type="button" onClick={() => navigator.clipboard.writeText(location).catch(() => setError("Unable to copy location."))}>Copy location</button>
    <pre>{item.snippet}</pre>{item.truncated && <small>Saved excerpt truncated</small>}
    {status && <p className="muted">Current source: {status.status.replaceAll("_", " ")}</p>}
    {status?.path && <a href={vscodeLocation(status.path,item.line,item.column)} onClick={event => {
      event.preventDefault();
      loadSourceStatus(`file:${item.path}`).then(current => {
        setStatus(current);
        if (current.path) window.location.assign(vscodeLocation(current.path,item.line,item.column));
      }).catch((error: unknown) => setError(error instanceof Error ? error.message : "Unable to check current source."));
    }}>Open current source in VS Code</a>}
    {error && <p role="alert">{error} <button onClick={() => {setError(null);setAttempt(n => n+1);}}>Retry</button></p>}
  </li>;
}

export function SourceEvidence({ edge, scope }: { edge: GraphEdge; scope: "canonical" | "package" }) {
  const [page, setPage] = useState<SourceEvidencePage | null>(null);
  const [offset, setOffset] = useState(0);
  const [attempt, setAttempt] = useState(0);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    loadSourceEvidence({kind:edge.kind,from:edge.from,to:edge.to}, scope, offset, controller.signal).then(result => {setPage(result);setError(null);}).catch((error: unknown) => {
      if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Unable to load evidence.");
    });
    return () => controller.abort();
  }, [edge.kind, edge.from, edge.to, scope, offset, attempt]);
  return <section className="detail-section">
    <h3>Saved source evidence {page && <span>{page.total}</span>}</h3>
    {error && <p role="alert">{error} <button onClick={() => setAttempt(n => n+1)}>Retry</button></p>}
    {!page && !error && <p>Loading evidence…</p>}
    {page && !page.recorded && <p className="muted">Source locations were not recorded for this relationship.</p>}
    {page && page.recorded && <>
      <ul className="evidence-list">{page.items.map((item,i) => <Occurrence key={`${item.path}:${item.line}:${item.column}:${i}`} item={item} />)}</ul>
      <p>Showing {Math.min(page.offset+1,page.total)}–{Math.min(page.offset+page.items.length,page.total)} of {page.total}</p>
      <button disabled={page.offset===0} onClick={() => setOffset(Math.max(0,page.offset-20))}>Previous</button>
      <button disabled={page.offset+20>=page.total} onClick={() => setOffset(page.offset+20)}>Next</button>
    </>}
  </section>;
}
