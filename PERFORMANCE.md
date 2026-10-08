# Saved graph performance investigation

Measured the existing saved yt-dlp snapshot: 1,046 files, 9,459 declarations,
17 packages, 18 graph cards and 76 folded imports. No analyzer, backend, storage
or canonical graph changes were needed. Existing uncommitted CLI and settings
work was preserved.

## What caused the lag

The footer counted folded imports, while React Flow rendered every Python
importer/provider line: **3,294 SVG edges and 26,779 DOM elements** behind 18
collapsed cards. Many routes overlapped exactly. During full-map pan/zoom, the
application recorded no React renders or graph computations, yet frame gaps
were hundreds of milliseconds. The measurements localize this bottleneck to
the browser's edge DOM/SVG rendering workload, rather than declaration count or
repeated source-graph computation. A native browser paint flame chart was not
captured.

Selection also replaced React Flow nodes with presentation objects lacking
`measured`. Five selections triggered 90 dimension changes and tens of thousands
of edge render attempts. Fresh empty file arrays and globally distributed
selection data invalidated memoized cards. Explorer and Inspector rebuilt
full-graph presentation maps on every parent render. Provider lookup rescanned
an import's entire evidence for each importer. Closed Inspector evidence
panels mounted their full evidence DOM.

Expansion exposed another ordering problem: edges requested new file handles
before React Flow registered them, flooding development logs with transient
missing-handle warnings.

## Measurements

Production, same saved scan, Firefox headless, 1440 × 1000, default Bézier lines
and grid. Pan/zoom restores the full-map viewport and uses trusted pointer/wheel
input. Five selections are scripted clicks. Their elapsed time includes driver
round trips, five 100 ms pauses and a final 1 s settling pause; it is **not** a
per-click input-latency measurement. Frame sampling runs for approximately
2.5 s; intervals under 1 ms at startup are excluded from percentiles.

| Collapsed map | Before | After |
| --- | ---: | ---: |
| Rendered edges | 3,294 | 76 |
| Initial DOM elements | 26,779 | 1,035 |
| Five-selection batch | 9,609 ms | 2,998 ms |
| Edge render attempts in that batch | 52,704 | 380 |
| Dimension changes in that batch | 90 | 0 |
| Full-graph presentation rebuilds in that batch | 12 | 0 |
| Pan/zoom median frame interval | 216.7 ms | 17.0 ms |
| Pan/zoom p95 frame interval | 333.4 ms | 17.3 ms |

Development's five-selection batch fell from 19,241 ms to 6,100 ms and edge
render attempts from 105,408 to 760. Strict Mode remains enabled; development
render counts include repeated render attempts, not just committed DOM work.
The final development expansion run recorded zero missing-handle warnings.

A terminal comparison across 16 expansion/filter states of the real snapshot
verified exact equality of visible graphs, Explorer trees and every line's ID,
endpoints and evidence. Graph/line computation took 1,798 ms before and 540 ms
after in the recorded benchmark run. These are single-run measurements, not a
statistical benchmark.

## Ten seconds idle

After initial rendering settled, **no instrumented operations repeated** in
production, either collapsed or with all cards expanded, before or after:

- no MapApp, GraphCanvas, MapNode, Explorer, Inspector or dependency-edge renders;
- no node synchronization, dimension changes, geometry measurements or explicit
  `updateNodeInternals` calls;
- no instrumented graph computations;
- no DOM mutations, bounding-rectangle reads or animation-frame requests or
  callbacks recorded by the browser probe.

The development application counters were also idle before the fixes. The
final development run additionally recorded zero browser-probe activity in
both idle windows. No MapNode/internals feedback loop was reproduced. These
probes do not measure native browser-process CPU, extensions or GPU activity;
the reported apparent idle CPU symptom remains unconfirmed.

## Changes and preservation

Collapsed routes paint their former topmost line. All evidence-backed line IDs
remain in presentation/Flow data; distinct handles restore distinct routes on
expansion. Mouse clicks retain their former topmost target. Collapsed duplicate
routes have fewer keyboard-focus targets, the tradeoff discussed during the
investigation. No canonical edges were aggregated, renamed or invented.

Node synchronization retains measurements and local drag positions while
honoring explicit position resets. Unchanged card data retains object identity;
selection data is scoped to the affected card. Dependency edges are memoized.
New file routes wait invisibly for registered handles and then restore their
original handle IDs. The readiness subscription compares handle-bound
references, so viewport changes do not repeatedly rebuild edge state.

Shared indexes cache containment, nodes, imports, adjacency, counts and
presentation profiles by immutable snapshot identity. Replacement snapshots
get separate indexes; weak keys allow old snapshots to be collected. Evidence
providers are grouped once per import. Explorer computations are memoized;
Inspector evidence rows mount when their native details panel opens. Go and
Python presentation tests continue to pass.

## Remaining limit

**The all-expanded map remains slow.** It paints 3,635 distinct file routes and
roughly 38,840 DOM elements. Production selections improved from 14,206 ms to
7,128 ms, but full-map pan/zoom still had a 225.0 ms median, 449.4 ms p95 and a
699.6 ms maximum frame interval. Development's expanded-map p95 was 517.1 ms.
This investigation does not establish smooth interaction with all file routes
exposed simultaneously. Reducing that remaining SVG workload requires a
separate rendering/interaction decision; this change preserves those routes.

## Reproduce and inspect

Open the frontend with `?perf=1`. In its console:

```js
window.namiPerformance.reset();
// Leave the browser untouched for ten seconds, then inspect:
window.namiPerformance.snapshot();
```

Instrumentation is opt-in. Normal use installs no profiling timers, observers
or logging. `web/scripts/profile.mjs` additionally instruments browser DOM and
geometry work in its temporary page. It needs the existing Firefox WebDriver
BiDi server, not a newly installed browser package:

```sh
mkdir -p /tmp/nami-perf-firefox
firefox --headless --no-remote --profile /tmp/nami-perf-firefox \
  --remote-debugging-port 9222 about:blank
# In another terminal, with the saved-scan API and frontend already running:
NAMI_PROFILE_URL='http://localhost:3000/?perf=1' \
  node web/scripts/profile.mjs /tmp/nami-profile.json
```

Raw accepted samples are in `web/performance/`. The original development
baseline predates the browser-wide probe and pan workload; its idle counters
cover application operations only. Earlier failed driver runs (wrong development
origin and bulk browser-side click loops that stalled or killed content
processes) were excluded. The final driver expands cards individually.

`web/scripts/benchmark.mjs` accepts saved `/api/v1/graph` and `/api/v1/packages`
JSON files, plus an optional baseline library directory, and fails on any graph,
line, Explorer or canonical-fact mismatch.

Validation passed: `make check build` with a temporary Go cache, 34 frontend
tests, TypeScript, ESLint, production build and `git diff --check`. The sandboxed
Go check initially hit a read-only cache; the sandboxed Next build failed to
read its spawned TypeScript configuration. Both checks passed outside the
sandbox. Visual acceptance remains manual: inspect line selection, expansion,
file-list scrolling, settings, impact and dragged-position preservation on the
saved scan and a Go scan. Passing these terminal checks alone does not prove
that all reported performance symptoms are resolved.
