import assert from "node:assert/strict";
import test from "node:test";
import { parsePreferences, setPreferences } from "./preferences.ts";

test("invalid saved preferences fall back independently", () => {
  const defaults = { theme: "system", lineStyle: "bezier", showGrid: true };
  for (const raw of [null, "broken", "null", "42", '"dark"', "[]"]) {
    assert.deepEqual(parsePreferences(raw), defaults);
  }
  assert.deepEqual(parsePreferences('{"theme":"dark","lineStyle":"unknown","showGrid":"false"}'), { ...defaults, theme: "dark" });
  assert.deepEqual(parsePreferences('{"theme":"light","lineStyle":"stepped","showGrid":false}'), { theme: "light", lineStyle: "stepped", showGrid: false });
});

test("saving preserves the legacy theme and other choices, including after a failed save", () => {
  const previousStorage = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  const previousWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
  const saved = new Map<string, string>([["nami-theme", "dark"]]);
  const events = new EventTarget();
  let blocked = false;
  let notifications = 0;
  events.addEventListener("nami-preferences", () => notifications++);
  Object.defineProperty(globalThis, "window", { configurable: true, value: events });
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: {
    getItem(key: string) { return saved.get(key) ?? null; },
    setItem(key: string, value: string) {
      if (blocked) throw new Error("Storage blocked");
      saved.set(key, value);
    },
  } });
  try {
    setPreferences({ lineStyle: "straight" });
    assert.deepEqual(parsePreferences(saved.get("nami-preferences")!), { theme: "dark", lineStyle: "straight", showGrid: true });
    blocked = true;
    assert.doesNotThrow(() => setPreferences({ showGrid: false }));
    assert.equal(parsePreferences(saved.get("nami-preferences")!).showGrid, true);
    blocked = false;
    setPreferences({ theme: "light" });
    assert.deepEqual(parsePreferences(saved.get("nami-preferences")!), { theme: "light", lineStyle: "straight", showGrid: false });
    assert.equal(notifications, 3);
  } finally {
    if (previousStorage) Object.defineProperty(globalThis, "localStorage", previousStorage);
    else Reflect.deleteProperty(globalThis, "localStorage");
    if (previousWindow) Object.defineProperty(globalThis, "window", previousWindow);
    else Reflect.deleteProperty(globalThis, "window");
  }
});
