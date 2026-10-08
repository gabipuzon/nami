// Opt in with ?perf=1. No observers, timers or logging run in normal use.
export interface PerformanceMetric { calls: number; totalMs: number; maxMs: number }
const metrics = new Map<string, PerformanceMetric>();
const counts = new Map<string, number>();
let enabled = false;
export const performanceProbe = {
  enable() { enabled = true; },
  reset() { metrics.clear(); },
  counts() { return Object.fromEntries(counts); },
  snapshot() { return Object.fromEntries([...metrics].map(([name, value]) => [name, { ...value }])); },
};
declare global { interface Window { namiPerformance: typeof performanceProbe } }
if (typeof window !== "undefined" && new URLSearchParams(window.location.search).get("perf") === "1") {
  enabled = true;
  window.namiPerformance = performanceProbe;
}
export function recordPerformance(name: string, duration = 0): void {
  if (!enabled) return;
  const metric = metrics.get(name) ?? { calls: 0, totalMs: 0, maxMs: 0 };
  metric.calls++;
  metric.totalMs += duration;
  metric.maxMs = Math.max(metric.maxMs, duration);
  metrics.set(name, metric);
}
export function measurePerformance<T>(name: string, operation: () => T): T {
  if (!enabled) return operation();
  const start = performance.now();
  try { return operation(); } finally { recordPerformance(name, performance.now() - start); }
}

// Current mounted/display counts survive timing resets and disappear on unmount.
export function setPerformanceCount(name: string, count: number | undefined): void {
  if (!enabled) return;
  if (count === undefined) counts.delete(name); else counts.set(name,count);
}
