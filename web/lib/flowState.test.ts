import assert from "node:assert/strict";
import test from "node:test";
import { Position, type Edge, type InternalNode, type Node } from "@xyflow/react";
import { edgesWithRegisteredHandles, equalHandleSnapshots, synchronizeNodes } from "./flowState.ts";

type TestNode = Node<{ label: string; selectedID: string | null }>;
const node = (id = "package:one"): TestNode => ({ id, position: { x: 10, y: 20 }, data: { label: id, selectedID: null } });

test("selection retains measured geometry and local drag position", () => {
  const before = node();
  const current = { ...before, position: { x: 80, y: 90 }, measured: { width: 304, height: 82 }, dragging: true };
  const next = { ...before, selected: true, data: { ...before.data, selectedID: before.id } };
  const [result] = synchronizeNodes([current], [next], [before]);
  assert.strictEqual(result.measured, current.measured);
  assert.strictEqual(result.position, current.position);
  assert.equal(result.dragging, true);
  assert.equal(result.selected, true);
  assert.equal(result.data.selectedID, before.id);
  assert.equal(before.selected, undefined);
});

test("unchanged presentation data retains identity for memoized cards", () => {
  const before = node();
  const current = { ...before, measured: { width: 304, height: 82 } };
  const [result] = synchronizeNodes([current], [{ ...before, data: { ...before.data } }], [before]);
  assert.strictEqual(result.data, current.data);
});

test("explicit position reset takes precedence over Flow drag state", () => {
  const before = { ...node(), position: { x: 80, y: 90 } };
  const current = { ...before, position: { x: 120, y: 140 }, measured: { width: 304, height: 400 } };
  const next = node();
  const [result] = synchronizeNodes([current], [next], [before]);
  assert.strictEqual(result.position, next.position);
  assert.strictEqual(result.measured, current.measured);
});

test("structural changes add and remove cards without inheriting another card's geometry", () => {
  const before = node();
  const next = node("package:new");
  const result = synchronizeNodes([{ ...before, measured: { width: 304, height: 82 } }], [next], [before]);
  assert.deepEqual(result, [next]);
  assert.equal(result[0].measured, undefined);
});

test("initial synchronization is a no-op", () => {
  const initial = [node()];
  assert.strictEqual(synchronizeNodes(initial, initial, initial), initial);
});


test("file routes wait for handle registration and restore their original identity and endpoints", () => {
  const edge: Edge = { id: "line:exact", source: "supplier", target: "importer", sourceHandle: "s.py", targetHandle: "i.py", data: { canonicalEdgeID: "IMPORTS:i.py->s.py" } };
  const handles = (id: string, type: "source" | "target"): NonNullable<InternalNode["internals"]["handleBounds"]> => ({
    source: type === "source" ? [{ id, type, nodeId: "supplier", position: Position.Right, x: 0, y: 0, width: 1, height: 1 }] : [],
    target: type === "target" ? [{ id, type, nodeId: "importer", position: Position.Left, x: 0, y: 0, width: 1, height: 1 }] : [],
  });
  const edges = [edge];
  const pending = edgesWithRegisteredHandles(edges, []);
  assert.equal(pending[0].hidden, true);
  assert.equal(pending[0].id, edge.id);
  assert.strictEqual(pending[0].data, edge.data);
  assert.equal(edge.hidden, undefined);
  const partial = edgesWithRegisteredHandles(edges, [["supplier", handles("s.py", "source")]]);
  assert.equal(partial[0].sourceHandle, "s.py");
  assert.equal(partial[0].targetHandle, undefined);
  const ready = edgesWithRegisteredHandles(edges, [["supplier", handles("s.py", "source")], ["importer", handles("i.py", "target")]]);
  assert.strictEqual(ready, edges);
  assert.deepEqual(ready[0], edge);
});

test("viewport changes with unchanged handle bounds do not resynchronize edges", () => {
  const bounds = { source: [], target: [] };
  assert.ok(equalHandleSnapshots([["one", bounds]], [["one", bounds]]));
  assert.ok(!equalHandleSnapshots([["one", bounds]], [["one", { source: [], target: [] }]]));
  const folded: Edge[] = [{ id: "line", source: "one", target: "two", hidden: true }];
  assert.strictEqual(edgesWithRegisteredHandles(folded, []), folded);
});
