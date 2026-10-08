import { useMemo, useSyncExternalStore } from "react";

export type Theme = "system" | "light" | "dark";
export type LineStyle = "bezier" | "straight" | "stepped";
export interface Preferences {
  theme: Theme;
  lineStyle: LineStyle;
  showGrid: boolean;
}

const defaults: Preferences = { theme: "system", lineStyle: "bezier", showGrid: true };
const defaultSnapshot = JSON.stringify(defaults);
const storageKey = "nami-preferences";
const changeEvent = "nami-preferences";
let unsavedSnapshot: string | null = null;

export function parsePreferences(raw: string | null): Preferences {
  let value: unknown;
  try { value = raw === null ? null : JSON.parse(raw); } catch { return { ...defaults }; }
  if (typeof value !== "object" || value === null) return { ...defaults };
  const stored = value as Record<string, unknown>;
  return {
    theme: stored.theme === "light" || stored.theme === "dark" ? stored.theme : "system",
    lineStyle: stored.lineStyle === "straight" || stored.lineStyle === "stepped" ? stored.lineStyle : "bezier",
    showGrid: typeof stored.showGrid === "boolean" ? stored.showGrid : true,
  };
}

function readSnapshot(): string {
  // A failed save still applies to this page without claiming persistence.
  if (unsavedSnapshot !== null) return unsavedSnapshot;
  try {
    return localStorage.getItem(storageKey) ?? JSON.stringify({ theme: localStorage.getItem("nami-theme") });
  } catch { return defaultSnapshot; }
}

function subscribe(callback: () => void): () => void {
  window.addEventListener(changeEvent, callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener(changeEvent, callback);
    window.removeEventListener("storage", callback);
  };
}

export function setPreferences(patch: Partial<Preferences>): void {
  const snapshot = JSON.stringify({ ...parsePreferences(readSnapshot()), ...patch });
  try {
    localStorage.setItem(storageKey, snapshot);
    unsavedSnapshot = null;
  } catch { unsavedSnapshot = snapshot; }
  window.dispatchEvent(new Event(changeEvent));
}

export function usePreferences(): { preferences: Preferences; saveFailed: boolean } {
  const snapshot = useSyncExternalStore(subscribe, readSnapshot, () => defaultSnapshot);
  const saveFailed = useSyncExternalStore(subscribe, () => unsavedSnapshot !== null, () => false);
  const preferences = useMemo(() => parsePreferences(snapshot), [snapshot]);
  return { preferences, saveFailed };
}
