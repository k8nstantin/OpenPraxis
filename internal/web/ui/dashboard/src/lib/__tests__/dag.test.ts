// Ports the 9 fixtures from internal/web/ui/views/__tests__/product-dag.test.js
// to vitest. Locks the same three invariants from
// dag-renderer-recurring-failures.md so the renderer can't drift back
// into the "tangled spaghetti" failure mode.

import { describe, expect, it } from 'vitest';
import { buildDagElements, type DagElement, type DagEdgeType } from '../dag';
import type { ProductHierarchy } from '../types';

function elementsByKind(elements: DagElement[]) {
  const nodes = elements.filter((e) => e.data.id && !e.data.source);
  const edges = elements.filter((e) => e.data.source);
  return { nodes, edges };
}
const edgesOfType = (edges: DagElement[], type: DagEdgeType) =>
  edges.filter((e) => e.data.edgeType === type);

describe('buildDagElements', () => {
  it('fixture 1: linear chain product → M1 → M2 → M3 with 1 task each', () => {
    const data: ProductHierarchy = {
      id: 'p1', marker: 'p1', title: 'P1', status: 'open', meta: {},
      children: [
        { id: 'm1', marker: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
          children: [{ id: 't1', marker: 't1', title: 'T1 task', status: 'waiting', meta: {}, depends_on: '' }] },
        { id: 'm2', marker: 'm2', title: 'M2', status: 'open', meta: {}, depends_on: 'm1',
          children: [{ id: 't2', marker: 't2', title: 'T2 task', status: 'waiting', meta: {}, depends_on: '' }] },
        { id: 'm3', marker: 'm3', title: 'M3', status: 'open', meta: {}, depends_on: 'm2',
          children: [{ id: 't3', marker: 't3', title: 'T3 task', status: 'waiting', meta: {}, depends_on: '' }] },
      ],
    };
    const { nodes, edges } = elementsByKind(buildDagElements(data));
    expect(nodes.length).toBe(1 + 3 + 3);
    expect(edgesOfType(edges, 'product_link')).toHaveLength(1);
    expect(edgesOfType(edges, 'manifest_dep')).toHaveLength(2);
    expect(edgesOfType(edges, 'ownership')).toHaveLength(3);
  });

  it('fixture 2: fan-out — 1 manifest with 5 sibling tasks', () => {
    const data: ProductHierarchy = {
      id: 'p2', marker: 'p2', title: 'P2', status: 'open', meta: {},
      children: [
        { id: 'm1', marker: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
          children: ['t1', 't2', 't3', 't4', 't5'].map((id) => ({
            id, marker: id, title: id, status: 'waiting', meta: {}, depends_on: '',
          })) },
      ],
    };
    const { edges } = elementsByKind(buildDagElements(data));
    expect(edgesOfType(edges, 'product_link')).toHaveLength(1);
    expect(edgesOfType(edges, 'manifest_dep')).toHaveLength(0);
    expect(edgesOfType(edges, 'ownership')).toHaveLength(5);
    expect(edgesOfType(edges, 'task_dep')).toHaveLength(0);
  });

  it('fixture 3: empty manifest emits product + manifest + 1 product_link only', () => {
    const data: ProductHierarchy = {
      id: 'p3', marker: 'p3', title: 'P3', status: 'open', meta: {},
      children: [{ id: 'm1', marker: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '', children: [] }],
    };
    const { nodes, edges } = elementsByKind(buildDagElements(data));
    expect(nodes).toHaveLength(2);
    expect(edges).toHaveLength(1);
    expect(edges[0]!.data.edgeType).toBe('product_link');
  });

  it('fixture 4: task chain inside manifest — t2 depends_on t1', () => {
    const data: ProductHierarchy = {
      id: 'p4', marker: 'p4', title: 'P4', status: 'open', meta: {},
      children: [{
        id: 'm1', marker: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
        children: [
          { id: 't1', marker: 't1', title: 'first', status: 'waiting', meta: {}, depends_on: '' },
          { id: 't2', marker: 't2', title: 'second', status: 'waiting', meta: {}, depends_on: 't1' },
        ],
      }],
    };
    const { edges } = elementsByKind(buildDagElements(data));
    expect(edgesOfType(edges, 'ownership')).toHaveLength(1);
    const taskDeps = edgesOfType(edges, 'task_dep');
    expect(taskDeps).toHaveLength(1);
    expect(taskDeps[0]!.data.source).toBe('t1');
    expect(taskDeps[0]!.data.target).toBe('t2');
  });

  it('fixture 5: long task title is truncated to ≤36 chars with ellipsis', () => {
    const longTitle = 'this is a very long task title that should definitely be truncated by the renderer';
    const data: ProductHierarchy = {
      id: 'p5', marker: 'p5', title: 'P5', status: 'open', meta: {},
      children: [{
        id: 'm1', marker: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '',
        children: [{ id: 't1', marker: 't1', title: longTitle, status: 'waiting', meta: {}, depends_on: '' }],
      }],
    };
    const els = buildDagElements(data);
    const taskNode = els.find((e) => e.data.id === 't1');
    expect(taskNode).toBeTruthy();
    expect(taskNode!.data.label!.length).toBeLessThanOrEqual(36);
    expect(taskNode!.data.label!.endsWith('…')).toBe(true);
    expect(taskNode!.data.title).toBe(longTitle);
  });

  it('fixture 6: multi-root product — both M1 and M3 are roots', () => {
    const data: ProductHierarchy = {
      id: 'p6', marker: 'p6', title: 'P6', status: 'open', meta: {},
      children: [
        { id: 'm1', marker: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '', children: [] },
        { id: 'm2', marker: 'm2', title: 'M2', status: 'open', meta: {}, depends_on: 'm1', children: [] },
        { id: 'm3', marker: 'm3', title: 'M3', status: 'open', meta: {}, depends_on: '', children: [] },
      ],
    };
    const { edges } = elementsByKind(buildDagElements(data));
    expect(edgesOfType(edges, 'product_link')).toHaveLength(2);
    expect(edgesOfType(edges, 'manifest_dep')).toHaveLength(1);
  });

  it('fixture 7: cross-product depends_on does not orphan nodes', () => {
    const data: ProductHierarchy = {
      id: 'p7', marker: 'p7', title: 'P7', status: 'open', meta: {},
      children: [
        { id: 'm1', marker: 'm1', title: 'M1', status: 'open', meta: {}, depends_on: '', children: [] },
        { id: 'm2', marker: 'm2', title: 'M2', status: 'open', meta: {}, depends_on: 'external-kernel-id', children: [] },
        { id: 'm3', marker: 'm3', title: 'M3', status: 'open', meta: {}, depends_on: 'm1', children: [] },
      ],
    };
    const { nodes, edges } = elementsByKind(buildDagElements(data));
    const nodeIds = new Set(nodes.map((n) => n.data.id));
    edges.forEach((e) => {
      expect(nodeIds.has(e.data.source!)).toBe(true);
      expect(nodeIds.has(e.data.target!)).toBe(true);
    });
    const productLinks = edgesOfType(edges, 'product_link');
    const targets = new Set(productLinks.map((e) => e.data.target));
    expect(targets.has('m1')).toBe(true);
    expect(targets.has('m2')).toBe(true);
    expect(targets.has('m3')).toBe(false);
    expect(edgesOfType(edges, 'manifest_dep')).toHaveLength(1);
  });

  it('fixture 8: umbrella with sub_products', () => {
    const data: ProductHierarchy = {
      id: 'umb', marker: 'umb', title: 'Umbrella', status: 'open', meta: {},
      children: [],
      sub_products: [
        { id: 'sub1', marker: 'sub1', title: 'Sub1', status: 'open', meta: {},
          children: [{ id: 'm1', marker: 'm1', title: 'S1M1', status: 'open', meta: {}, depends_on: '',
            children: [{ id: 't1', marker: 't1', title: 'S1T1', status: 'waiting', meta: {}, depends_on: '' }] }] },
        { id: 'sub2', marker: 'sub2', title: 'Sub2', status: 'open', meta: {},
          children: [{ id: 'm2', marker: 'm2', title: 'S2M1', status: 'open', meta: {}, depends_on: '',
            children: [{ id: 't2', marker: 't2', title: 'S2T1', status: 'waiting', meta: {}, depends_on: '' }] }] },
      ],
    };
    const { nodes, edges } = elementsByKind(buildDagElements(data));
    const productNodes = nodes.filter((n) => n.data.type === 'product');
    expect(productNodes).toHaveLength(3);
    const umbrellaEdges = edges.filter((e) => e.data.source === 'umb');
    expect(umbrellaEdges).toHaveLength(2);
    const sub1ManEdges = edges.filter((e) => e.data.source === 'sub1' && e.data.target === 'm1');
    const sub2ManEdges = edges.filter((e) => e.data.source === 'sub2' && e.data.target === 'm2');
    expect(sub1ManEdges).toHaveLength(1);
    expect(sub2ManEdges).toHaveLength(1);
    expect(nodes.find((n) => n.data.id === 't1')).toBeTruthy();
    expect(nodes.find((n) => n.data.id === 't2')).toBeTruthy();
  });

  it('fixture 9: cyclic sub_products do not infinite-recurse', () => {
    const data: ProductHierarchy = {
      id: 'umb', marker: 'umb', title: 'Umb', status: 'open', meta: {},
      children: [],
      sub_products: [
        { id: 'sub1', marker: 'sub1', title: 'Sub1', status: 'open', meta: {}, children: [],
          sub_products: [{ id: 'umb', marker: 'umb', title: 'Umb', status: 'open', meta: {}, children: [] }] },
      ],
    };
    const { nodes } = elementsByKind(buildDagElements(data));
    expect(nodes.filter((n) => n.data.id === 'umb')).toHaveLength(1);
  });
});
