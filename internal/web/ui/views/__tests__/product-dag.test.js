// Pure-Node regression test for product-dag.js — locks the three invariants
// from memory dag-renderer-recurring-failures.md so the renderer can't drift
// back into the "tangled spaghetti" failure mode without the test failing.
//
// Run: node internal/web/ui/views/__tests__/product-dag.test.js
// Wired into `make test-ui` (see Makefile). No npm deps, no jsdom, no
// cytoscape — buildDagElements is a pure function once we shim `window`.
'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');
const vm = require('vm');

// Shim a minimal `window.OL` and load the renderer source. The IIFE attaches
// buildDagElements to OL — that's all this test needs (no cytoscape/dagre
// required because we're testing the EDGE MODEL, not the laid-out coordinates).
const OL = {};
const ctx = vm.createContext({ window: { OL }, console });
const src = fs.readFileSync(path.join(__dirname, '..', 'product-dag.js'), 'utf8');
vm.runInContext(src, ctx);
assert.ok(typeof OL.buildDagElements === 'function', 'buildDagElements not exported');

function elementsByKind(elements) {
  const nodes = elements.filter(e => e.data && e.data.id && !e.data.source);
  const edges = elements.filter(e => e.data && e.data.source);
  return { nodes, edges };
}

function edgesOfType(edges, type) {
  return edges.filter(e => e.data.edgeType === type);
}

// ─── Fixture 1: linear chain ────────────────────────────────────────────────
// product → M1 → M2 → M3, one task per manifest. Tests:
//   - exactly 1 product_link edge (only M1 is a root)
//   - 2 manifest_dep edges (M1→M2, M2→M3)
//   - 3 ownership edges (one per orphan task)
{
  const data = {
    id: 'p1', title: 'P1', status: 'open', meta: {},
    children: [
      { id: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
        children: [{ id: 't1', title: 'T1 task', status: 'waiting', meta: {}, depends_on: '' }] },
      { id: 'm2', title: 'M2', status: 'open', meta: {}, depends_on: 'm1',
        children: [{ id: 't2', title: 'T2 task', status: 'waiting', meta: {}, depends_on: '' }] },
      { id: 'm3', title: 'M3', status: 'open', meta: {}, depends_on: 'm2',
        children: [{ id: 't3', title: 'T3 task', status: 'waiting', meta: {}, depends_on: '' }] },
    ],
  };
  const { nodes, edges } = elementsByKind(OL.buildDagElements(data));
  assert.strictEqual(nodes.length, 1 + 3 + 3, 'linear: node count');
  assert.strictEqual(edgesOfType(edges, 'product_link').length, 1, 'linear: product_link only for root M1');
  assert.strictEqual(edgesOfType(edges, 'manifest_dep').length, 2, 'linear: manifest chain edges');
  assert.strictEqual(edgesOfType(edges, 'ownership').length, 3, 'linear: ownership edges for orphan tasks');
}

// ─── Fixture 2: fan-out (one manifest, 5 sibling tasks, no deps) ────────────
// Tests that ownership edges fire for every task when none have depends_on.
{
  const data = {
    id: 'p2', title: 'P2', status: 'open', meta: {},
    children: [
      { id: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
        children: [
          { id: 't1', title: 'A', status: 'waiting', meta: {}, depends_on: '' },
          { id: 't2', title: 'B', status: 'waiting', meta: {}, depends_on: '' },
          { id: 't3', title: 'C', status: 'waiting', meta: {}, depends_on: '' },
          { id: 't4', title: 'D', status: 'waiting', meta: {}, depends_on: '' },
          { id: 't5', title: 'E', status: 'waiting', meta: {}, depends_on: '' },
        ] },
    ],
  };
  const { edges } = elementsByKind(OL.buildDagElements(data));
  assert.strictEqual(edgesOfType(edges, 'product_link').length, 1, 'fan-out: product_link');
  assert.strictEqual(edgesOfType(edges, 'manifest_dep').length, 0, 'fan-out: no manifest deps');
  assert.strictEqual(edgesOfType(edges, 'ownership').length, 5, 'fan-out: ownership per task');
  assert.strictEqual(edgesOfType(edges, 'task_dep').length, 0, 'fan-out: no task deps');
}

// ─── Fixture 3: empty manifest ──────────────────────────────────────────────
// Manifest with zero tasks must not crash and must not emit phantom edges.
{
  const data = {
    id: 'p3', title: 'P3', status: 'open', meta: {},
    children: [{ id: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '', children: [] }],
  };
  const { nodes, edges } = elementsByKind(OL.buildDagElements(data));
  assert.strictEqual(nodes.length, 2, 'empty: product + manifest');
  assert.strictEqual(edges.length, 1, 'empty: only product_link');
  assert.strictEqual(edges[0].data.edgeType, 'product_link', 'empty: edge is product_link');
}

// ─── Fixture 4: task chain inside manifest ──────────────────────────────────
// T2 depends_on T1 (same manifest) → ownership edge for T1, task_dep for T2.
{
  const data = {
    id: 'p4', title: 'P4', status: 'open', meta: {},
    children: [
      { id: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
        children: [
          { id: 't1', title: 'first', status: 'waiting', meta: {}, depends_on: '' },
          { id: 't2', title: 'second', status: 'waiting', meta: {}, depends_on: 't1' },
        ] },
    ],
  };
  const { edges } = elementsByKind(OL.buildDagElements(data));
  assert.strictEqual(edgesOfType(edges, 'ownership').length, 1, 'chain: only T1 gets ownership');
  assert.strictEqual(edgesOfType(edges, 'task_dep').length, 1, 'chain: T2 gets task_dep');
  const taskDep = edgesOfType(edges, 'task_dep')[0];
  assert.strictEqual(taskDep.data.source, 't1', 'chain: task_dep source is parent');
  assert.strictEqual(taskDep.data.target, 't2', 'chain: task_dep target is child');
}

// ─── Fixture 5: long task title is truncated ────────────────────────────────
// Invariant #3 from the memory: node width must bound label width. The
// truncation step in buildDagElements must cap labels at 36 chars (current
// node width 120px × ~2 lines @ font-size 9px) so labels can't overflow.
{
  const longTitle = 'this is a very long task title that should definitely be truncated by the renderer';
  const data = {
    id: 'p5', title: 'P5', status: 'open', meta: {},
    children: [{ id: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
      children: [{ id: 't1', title: longTitle, status: 'waiting', meta: {}, depends_on: '' }] }],
  };
  const elements = OL.buildDagElements(data);
  const taskNode = elements.find(e => e.data && e.data.id === 't1');
  assert.ok(taskNode, 'long-title: task node present');
  assert.ok(taskNode.data.label.length <= 36, 'long-title: label ≤ 36 chars (got ' + taskNode.data.label.length + ')');
  assert.ok(taskNode.data.label.endsWith('…'), 'long-title: ellipsis marker present');
  assert.strictEqual(taskNode.data.title, longTitle, 'long-title: full title preserved on data.title');
}

// ─── Fixture 6: multi-root product (two independent manifest chains) ────────
// Both M1 and M3 are roots; product_link must fire for both. Verifies the
// pruning rule isn't over-eager.
{
  const data = {
    id: 'p6', title: 'P6', status: 'open', meta: {},
    children: [
      { id: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '', children: [] },
      { id: 'm2', title: 'M2', status: 'open', meta: {}, depends_on: 'm1', children: [] },
      { id: 'm3', title: 'M3', status: 'open', meta: {}, depends_on: '', children: [] },
    ],
  };
  const { edges } = elementsByKind(OL.buildDagElements(data));
  assert.strictEqual(edgesOfType(edges, 'product_link').length, 2, 'multi-root: product_link for M1 and M3');
  assert.strictEqual(edgesOfType(edges, 'manifest_dep').length, 1, 'multi-root: one chain edge M1→M2');
}

// ─── Fixture 7: cross-product depends_on (PR #224) ──────────────────────────
// M2's depends_on points at a manifest NOT in this product (e.g. M2 lives
// under AOS/Execution but depends on M6 in AOS/Kernel). Invariants:
//   - no edge with a missing source node (would crash cytoscape)
//   - M2 still reachable from the root via a product_link ownership edge
//   - in-product deps (M1→M3) still emit normally
{
  const data = {
    id: 'p7', title: 'P7', status: 'open', meta: {},
    children: [
      { id: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '', children: [] },
      { id: 'm2', title: 'M2', status: 'open', meta: {}, depends_on: 'external-kernel-id', children: [] },
      { id: 'm3', title: 'M3', status: 'open', meta: {}, depends_on: 'm1', children: [] },
    ],
  };
  const { nodes, edges } = elementsByKind(OL.buildDagElements(data));
  const nodeIds = new Set(nodes.map(n => n.data.id));
  edges.forEach(e => {
    assert.ok(nodeIds.has(e.data.source), 'cross-product: edge source must exist as node (got ' + e.data.source + ')');
    assert.ok(nodeIds.has(e.data.target), 'cross-product: edge target must exist as node (got ' + e.data.target + ')');
  });
  const productLinks = edgesOfType(edges, 'product_link');
  const targets = new Set(productLinks.map(e => e.data.target));
  assert.ok(targets.has('m1'), 'cross-product: M1 gets product_link');
  assert.ok(targets.has('m2'), 'cross-product: M2 with external-only deps gets product_link (keeps it reachable)');
  assert.ok(!targets.has('m3'), 'cross-product: M3 with in-product dep does not get redundant product_link');
  assert.strictEqual(edgesOfType(edges, 'manifest_dep').length, 1, 'cross-product: only in-product dep M1→M3 emitted');
}

// ─── Fixture 8: umbrella with sub_products (PR #225) ────────────────────────
// Umbrella product owns zero manifests, has 2 sub-products, each with 1
// manifest + 1 task. Invariants:
//   - all 3 product nodes emitted (umbrella + 2 subs)
//   - 2 product_link edges from umbrella → each sub
//   - each sub's manifests + tasks present and correctly linked
{
  const data = {
    id: 'umb', title: 'Umbrella', status: 'open', meta: {},
    children: [],
    sub_products: [
      { id: 'sub1', title: 'Sub1', status: 'open', meta: {},
        children: [{ id: 'm1', title: 'S1M1', status: 'open', meta: {}, depends_on: '',
          children: [{ id: 't1', title: 'S1T1', status: 'waiting', meta: {}, depends_on: '' }] }] },
      { id: 'sub2', title: 'Sub2', status: 'open', meta: {},
        children: [{ id: 'm2', title: 'S2M1', status: 'open', meta: {}, depends_on: '',
          children: [{ id: 't2', title: 'S2T1', status: 'waiting', meta: {}, depends_on: '' }] }] },
    ],
  };
  const { nodes, edges } = elementsByKind(OL.buildDagElements(data));
  const productNodes = nodes.filter(n => n.data.type === 'product');
  assert.strictEqual(productNodes.length, 3, 'umbrella: 3 product nodes (umbrella + 2 subs)');
  const umbrellaEdges = edges.filter(e => e.data.source === 'umb');
  assert.strictEqual(umbrellaEdges.length, 2, 'umbrella: 2 outgoing edges (one per sub)');
  const sub1ManEdges = edges.filter(e => e.data.source === 'sub1' && e.data.target === 'm1');
  const sub2ManEdges = edges.filter(e => e.data.source === 'sub2' && e.data.target === 'm2');
  assert.strictEqual(sub1ManEdges.length, 1, 'umbrella: sub1 → m1 ownership');
  assert.strictEqual(sub2ManEdges.length, 1, 'umbrella: sub2 → m2 ownership');
  assert.ok(nodes.find(n => n.data.id === 't1'), 'umbrella: t1 emitted');
  assert.ok(nodes.find(n => n.data.id === 't2'), 'umbrella: t2 emitted');
}

// ─── Fixture 9: cyclic sub_products (defense in depth) ──────────────────────
// Malformed payload where a sub-product references an ancestor. Seen-set
// must prevent infinite recursion / duplicate node emission.
{
  const data = {
    id: 'umb', title: 'Umb', status: 'open', meta: {},
    children: [],
    sub_products: [
      { id: 'sub1', title: 'Sub1', status: 'open', meta: {}, children: [],
        sub_products: [{ id: 'umb', title: 'Umb', status: 'open', meta: {}, children: [] }] },
    ],
  };
  const { nodes } = elementsByKind(OL.buildDagElements(data));
  const umbNodes = nodes.filter(n => n.data.id === 'umb');
  assert.strictEqual(umbNodes.length, 1, 'cycle-defense: umbrella emitted exactly once');
}

console.log('product-dag.test.js: all 9 fixtures passed');
