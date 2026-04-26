import type { ProductHierarchy, Manifest, Task } from './types';

// buildDagElements ports the edge-pruning + truncation logic from
// the legacy views/product-dag.js verbatim. Cytoscape consumes the
// returned { nodes, edges } directly. The function is pure so it can
// be unit-tested without touching the DOM (see __tests__/dag.test.ts).

export type DagEdgeType = 'product_link' | 'manifest_dep' | 'ownership' | 'task_dep';
export type DagNodeType = 'product' | 'manifest' | 'task';

export interface DagElement {
  data: {
    id?: string;
    label?: string;
    title?: string;
    type?: DagNodeType;
    status?: string;
    marker?: string;
    depends_on?: string;
    meta?: string;
    taskInfo?: string;
    source?: string;
    target?: string;
    edgeType?: DagEdgeType;
  };
}

function shortLabel(title: string): string {
  return title.replace(/^QA\s+/, '').replace(/^OpenPraxis\s+/, '').replace(/\s*—\s*.+$/, '');
}

export function buildDagElements(data: ProductHierarchy | null | undefined): DagElement[] {
  const elements: DagElement[] = [];
  if (!data) return elements;
  const seenProducts = new Set<string>();

  function addProduct(p: ProductHierarchy): void {
    if (!p || seenProducts.has(p.id)) return;
    seenProducts.add(p.id);

    elements.push({
      data: {
        id: p.id,
        label: p.title,
        title: p.title,
        type: 'product',
        status: p.status,
        marker: p.marker,
        meta: JSON.stringify(p.meta || {}),
      },
    });

    const manifests: Manifest[] = p.children || [];
    const manifestIds = new Set(manifests.map((m) => m.id));

    // Product → manifest ownership edges. Emit only when the manifest
    // has no in-product dep that would otherwise connect it (avoids the
    // 2026-04-23 "tangled spaghetti"). Cross-product deps count as
    // external, so a manifest whose deps all point outside still gets an
    // ownership edge.
    for (const mown of manifests) {
      let inProductDeps = 0;
      if (mown.depends_on) {
        const ids = mown.depends_on.split(',').map((s) => s.trim()).filter(Boolean);
        for (const did of ids) if (manifestIds.has(did)) inProductDeps++;
      }
      if (inProductDeps === 0) {
        elements.push({ data: { source: p.id, target: mown.id, edgeType: 'product_link' } });
      }
    }

    for (const m of manifests) {
      const tasks: Task[] = m.children || [];
      const taskCount = tasks.length;
      const completedCount = tasks.filter((t) => t.status === 'completed').length;

      elements.push({
        data: {
          id: m.id,
          label: shortLabel(m.title),
          title: m.title,
          type: 'manifest',
          status: m.status,
          marker: m.marker,
          depends_on: m.depends_on || '',
          meta: JSON.stringify(m.meta || {}),
          taskInfo: `${completedCount}/${taskCount}`,
        },
      });

      if (m.depends_on) {
        const depIds = m.depends_on.split(',').map((s) => s.trim()).filter(Boolean);
        for (const di of depIds) {
          if (!manifestIds.has(di)) continue;
          elements.push({ data: { source: di, target: m.id, edgeType: 'manifest_dep' } });
        }
      }

      const taskIds = new Set(tasks.map((t) => t.id));

      for (const t of tasks) {
        const shortTitle = t.title.length > 36 ? t.title.substring(0, 35) + '…' : t.title;
        elements.push({
          data: {
            id: t.id,
            label: shortTitle,
            title: t.title,
            type: 'task',
            status: t.status,
            marker: t.marker,
            depends_on: t.depends_on || '',
            meta: JSON.stringify(t.meta || {}),
          },
        });

        if (t.depends_on && taskIds.has(t.depends_on)) {
          elements.push({ data: { source: t.depends_on, target: t.id, edgeType: 'task_dep' } });
        } else {
          elements.push({ data: { source: m.id, target: t.id, edgeType: 'ownership' } });
        }
      }
    }

    const subs = p.sub_products || [];
    for (const sub of subs) {
      elements.push({ data: { source: p.id, target: sub.id, edgeType: 'product_link' } });
      addProduct(sub);
    }
  }

  addProduct(data);
  return elements;
}
