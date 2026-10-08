// Existing Firefox WebDriver BiDi; no browser package dependency.
import { writeFile } from 'node:fs/promises';
const ws = new WebSocket('ws://127.0.0.1:9222/session');
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
let id = 0;
const pending = new Map();
ws.onmessage = ({ data }) => {
  const result = JSON.parse(data);
  const task = pending.get(result.id);
  if (!task) return;
  pending.delete(result.id);
  if (result.type === 'error') task.reject(new Error(JSON.stringify(result)));
  else task.resolve(result.result);
};
function call(method, params) { return new Promise((resolve, reject) => { pending.set(++id, { resolve, reject }); ws.send(JSON.stringify({ id, method, params })); }); }
await call('session.new', { capabilities: {} });

const { context } = await call('browsingContext.create', { type: 'tab' });
async function evaluate(expression) {
  const result = await call('script.evaluate', { expression, target: { context }, awaitPromise: true });
  if (result.type === 'exception') throw new Error(JSON.stringify(result));
  return result.result.value;
}
const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));
try {
  await call('browsingContext.setViewport', { context, viewport: { width: 1440, height: 1000 } });
  await call('browsingContext.navigate', { context, url: process.env.NAMI_PROFILE_URL ?? 'http://localhost:3000/?perf=1', wait: 'complete' });
  let ready = false;
  for (let attempt = 0; attempt < 120; attempt++) {
    if (await evaluate('!!window.namiPerformance && document.querySelectorAll(".react-flow__node").length > 0')) { ready = true; break; }
    await sleep(500);
  }
  if (!ready) throw new Error('Saved graph never rendered: ' + await evaluate('document.body.innerText'));
  await sleep(3000);
  await evaluate(`(() => {
    window.namiBrowserMetrics = {};
    const count = (name, amount = 1) => { window.namiBrowserMetrics[name] = (window.namiBrowserMetrics[name] ?? 0) + amount; };
    const rect = Element.prototype.getBoundingClientRect;
    Element.prototype.getBoundingClientRect = function (...args) { count('rectReads'); return rect.apply(this, args); };
    const raf = requestAnimationFrame;
    window.requestAnimationFrame = function (callback) { count('rafRequests'); return raf(now => { count('rafCallbacks'); callback(now); }); };
    const RO = ResizeObserver;
    window.ResizeObserver = class extends RO { constructor(callback) { super((entries, observer) => { count('resizeCallbacks'); callback(entries, observer); }); } };
    new MutationObserver(records => count('domMutations', records.length)).observe(document.documentElement, {subtree: true, childList: true, attributes: true, characterData: true});
  })()`);
  await evaluate(`(() => { const warn = console.warn; console.warn = (...args) => { if (String(args[0]).includes("Couldn't create edge")) { window.namiWarnings = (window.namiWarnings ?? 0) + 1; } warn(...args); }; })()`);
  const initial = JSON.parse(await evaluate('JSON.stringify({metrics: namiPerformance.snapshot(), nodes: document.querySelectorAll(".react-flow__node").length, edges: document.querySelectorAll(".react-flow__edge").length, dom: document.querySelectorAll("*").length, stats: document.querySelector(".explorer-secondary")?.textContent})'));
  const samples = {};
  console.log('initial', JSON.stringify(initial));
  for (const expanded of [false, true]) {
    if (expanded) {
      await evaluate('window.namiWarnings = 0');
      for (let i = 0; i < initial.nodes; i++) {
        await evaluate(`document.querySelectorAll(".map-node-toggle")[${i}]?.click()`);
        await sleep(100);
      }
      await sleep(3000);
      samples.expansionWarnings = await evaluate('window.namiWarnings ?? 0');
    }
    await evaluate('namiPerformance.reset(); window.namiWarnings = 0; window.namiBrowserMetrics = {}');
    await sleep(10000);
    samples[expanded ? 'expandedIdle' : 'collapsedIdle'] = JSON.parse(await evaluate('JSON.stringify({app: namiPerformance.snapshot(), browser: window.namiBrowserMetrics})'));
    console.log('idle', expanded, JSON.stringify(samples[expanded ? 'expandedIdle' : 'collapsedIdle']));
    await writeFile(process.argv[2] ?? '/tmp/nami-profile.json', JSON.stringify({ initial, samples }, null, 2));
    await evaluate('namiPerformance.reset(); window.namiWarnings = 0; window.namiBrowserMetrics = {}');
    const start = Date.now();
    // Scripted clicks measure render/computation work; not trusted-input latency.
    for (let i = 0; i < 5; i++) {
      await evaluate(`document.querySelectorAll(".tree-list > div > .tree-row .tree-label")[${i} % document.querySelectorAll(".tree-list > div > .tree-row .tree-label").length]?.click()`);
      await sleep(100);
    }
    await sleep(1000);
    samples[expanded ? 'expandedClicks' : 'collapsedClicks'] = { elapsedMs: Date.now() - start, metrics: JSON.parse(await evaluate('JSON.stringify(namiPerformance.snapshot())')), browser: JSON.parse(await evaluate('JSON.stringify(window.namiBrowserMetrics)')), warnings: await evaluate('window.namiWarnings ?? 0'), dom: await evaluate('document.querySelectorAll("*").length') };
    console.log('clicks', expanded, JSON.stringify(samples[expanded ? 'expandedClicks' : 'collapsedClicks']));
    await evaluate('document.querySelector(".react-flow__pane")?.click(); document.querySelector(".react-flow__controls-fitview")?.click()');
    await sleep(1500);
    await evaluate('namiPerformance.reset(); window.namiBrowserMetrics = {}');
    const point = JSON.parse(await evaluate('JSON.stringify((() => { const r = document.querySelector(".graph-stage").getBoundingClientRect(); for (const dx of [100, 160, 220]) for (const dy of [100, 160, 220]) { const x = Math.round(r.right - dx), y = Math.round(r.bottom - dy); const hit = document.elementFromPoint(x, y); if (hit?.classList.contains("react-flow__pane")) return {x, y}; } throw new Error("No empty pane point"); })())'));
    await evaluate('window.namiFrames = []; window.namiFrameEnd = performance.now() + 2500; window.namiFrameLast = performance.now(); requestAnimationFrame(function tick(now) { window.namiFrames.push(now - window.namiFrameLast); window.namiFrameLast = now; if (now < window.namiFrameEnd) requestAnimationFrame(tick); })');
    const actions = [{ type: 'pointerMove', x: point.x, y: point.y }, { type: 'pointerDown', button: 0 }];
    for (let i = 1; i <= 20; i++) actions.push({ type: 'pointerMove', x: point.x + i * 3, y: point.y + i * 2, duration: 16 });
    actions.push({ type: 'pointerUp', button: 0 });
    await call('input.performActions', { context, actions: [{ type: 'pointer', id: 'mouse', parameters: { pointerType: 'mouse' }, actions }] });
    await call('input.performActions', { context, actions: [{ type: 'wheel', id: 'wheel', actions: Array.from({length: 10}, (_, i) => ({ type: 'scroll', x: point.x, y: point.y, deltaX: 0, deltaY: i < 5 ? -30 : 30, duration: 16 })) }] });
    await sleep(2500);
    samples[expanded ? 'expandedPanZoom' : 'collapsedPanZoom'] = JSON.parse(await evaluate('JSON.stringify({metrics: namiPerformance.snapshot(), browser: window.namiBrowserMetrics, frames: window.namiFrames})'));
    const frameSample = samples[expanded ? 'expandedPanZoom' : 'collapsedPanZoom'];
    console.log('panZoom', expanded, JSON.stringify({metrics: frameSample.metrics, browser: frameSample.browser, frameCount: frameSample.frames.length}));
    await writeFile(process.argv[2] ?? '/tmp/nami-profile.json', JSON.stringify({ initial, samples }, null, 2));
  }
  const result = { url: process.env.NAMI_PROFILE_URL ?? 'http://localhost:3000', initial, samples };
  const output = process.argv[2] ?? '/tmp/nami-profile.json';
  await writeFile(output, JSON.stringify(result, null, 2));
  console.log('Saved profile:', output);
} catch (error) {
  console.error('Page after failure:', await evaluate('JSON.stringify({url: location.href, text: document.body?.innerText.slice(0,800), perf: !!window.namiPerformance})'));
  throw error;
} finally {
  await call('browsingContext.close', { context });
  await call('session.end', {});
  ws.close();
}
