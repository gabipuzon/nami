"use client";
import { useState, type ReactNode } from "react";

export function PageControls({ offset, total, onChange, disabled = false }: { offset: number; total: number; onChange: (offset: number) => void; disabled?: boolean }) {
  return <div className="page-controls"><span>Showing {total ? Math.min(offset+1,total) : 0}–{Math.min(offset+20,total)} of {total}</span><button type="button" disabled={disabled || offset === 0} onClick={() => onChange(Math.max(0,offset-20))}>Previous</button><button type="button" disabled={disabled || offset+20 >= total} onClick={() => onChange(offset+20)}>Next</button></div>;
}
export function PagedList<T>({ items, render }: { items: readonly T[]; render: (item: T, index: number) => ReactNode }) {
  const [page, setPage] = useState({ items, offset: 0 });
  const offset = page.items === items ? Math.min(page.offset, Math.max(0, Math.floor((items.length-1)/20)*20)) : 0;
  return <><li className="list-page-controls"><PageControls offset={offset} total={items.length} onChange={offset => setPage({items, offset})} /></li>{items.slice(offset,offset+20).map((item,index) => render(item,offset+index))}</>;
}
