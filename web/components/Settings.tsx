"use client";

import { useRef } from "react";
import { setPreferences, type Preferences } from "../lib/preferences";

export function Settings({ preferences, saveFailed }: { preferences: Preferences; saveFailed: boolean }) {
  const panel = useRef<HTMLDetailsElement>(null);
  return <details className="settings-control" ref={panel} onKeyDown={(event) => {
    if (event.key === "Escape" && panel.current?.open) {
      event.preventDefault();
      panel.current.open = false;
      panel.current.querySelector("summary")?.focus();
    }
  }}>
    <summary>Settings</summary>
    <section className="settings-panel" aria-label="Map preferences">
      <h2>Preferences</h2>
      <label className="settings-row"><span>Line style</span><select value={preferences.lineStyle} onChange={(event) => {
        const lineStyle = event.target.value;
        if (lineStyle === "bezier" || lineStyle === "straight" || lineStyle === "stepped") setPreferences({ lineStyle });
      }}><option value="bezier">Bézier</option><option value="straight">Straight</option><option value="stepped">Stepped</option></select></label>
      <label className="settings-row"><span>Theme</span><select value={preferences.theme} onChange={(event) => {
        const theme = event.target.value;
        if (theme === "system" || theme === "light" || theme === "dark") setPreferences({ theme });
      }}><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></label>
      <label className="settings-row"><span>Show grid</span><input type="checkbox" checked={preferences.showGrid} onChange={(event) => setPreferences({ showGrid: event.target.checked })} /></label>
      <p role="status">{saveFailed ? "Preferences could not be saved. They apply to this page only." : "Saved in this browser."}</p>
    </section>
  </details>;
}
