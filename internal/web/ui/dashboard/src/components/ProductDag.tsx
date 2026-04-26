import { useEffect, useRef } from 'react';
import { buildDagElements } from '../lib/dag';
import type { ProductHierarchy } from '../lib/types';

// Cytoscape lifecycle wrapper. Mounts a cytoscape instance against a
// ref'd <div>, destroys it on unmount, and reinitialises whenever the
// hierarchy changes. cytoscape + cytoscape-dagre are loaded lazily by
// React.lazy → this component, so they don't bloat the initial bundle.
//
// Loaded via dynamic import to avoid forcing the legacy /vendor/* CDN
// shape into our TS types — we just need a callable.

interface Props {
  data: ProductHierarchy | null | undefined;
  onNodeClick?: (id: string, type: string) => void;
}

interface CyHandle {
  destroy(): void;
  on(evt: string, sel: string, h: (e: { target: { data(): Record<string, unknown> } }) => void): void;
}
interface CytoscapeFactory {
  (opts: unknown): CyHandle;
  use?(ext: unknown): void;
}

let cyModulePromise: Promise<{ default: CytoscapeFactory }> | null = null;
let dagreModulePromise: Promise<{ default: unknown }> | null = null;

async function loadCytoscape(): Promise<CytoscapeFactory> {
  if (!cyModulePromise) {
    // @ts-expect-error — cytoscape ships its own types but we keep deps
    // light by not declaring them; the runtime contract is small.
    cyModulePromise = import('cytoscape');
    // @ts-expect-error — same as above for cytoscape-dagre.
    dagreModulePromise = import('cytoscape-dagre');
  }
  const [cyMod, dagreMod] = await Promise.all([cyModulePromise!, dagreModulePromise!]);
  if (cyMod.default.use) cyMod.default.use(dagreMod.default);
  return cyMod.default;
}

export default function ProductDag({ data, onNodeClick }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let destroyed = false;
    let cyInstance: CyHandle | null = null;

    if (!containerRef.current || !data) return;
    const elements = buildDagElements(data);

    loadCytoscape().then((cytoscape) => {
      if (destroyed || !containerRef.current) return;
      cyInstance = cytoscape({
        container: containerRef.current,
        elements,
        layout: {
          name: 'dagre',
          rankDir: 'TB',
          nodeSep: 40,
          rankSep: 90,
          edgeSep: 25,
          padding: 32,
          fit: true,
        },
        style: [
          { selector: 'node', style: {
            label: 'data(label)', 'text-wrap': 'wrap', 'text-overflow-wrap': 'anywhere',
            'text-max-width': '110px', 'font-size': '9px', 'text-valign': 'center', 'text-halign': 'center',
            color: '#e4e4e7', 'background-color': '#1a1a2e', 'border-width': 2, 'border-color': '#71717a',
            width: 120, height: 44, shape: 'round-rectangle', padding: '6px',
          } },
          { selector: 'node[type="product"]', style: { width: 180, height: 60, 'font-size': '12px', 'background-color': '#8b5cf6', 'border-color': '#8b5cf6', color: '#fff' } },
          { selector: 'node[type="manifest"]', style: { width: 140, height: 50, 'font-size': '10px', 'background-color': '#1e3a5f', 'border-color': '#3b82f6', 'border-width': 3 } },
          { selector: 'edge', style: { width: 1.5, 'line-color': 'rgba(255,255,255,0.15)', 'target-arrow-color': 'rgba(255,255,255,0.15)', 'target-arrow-shape': 'triangle', 'curve-style': 'straight', 'arrow-scale': 0.7 } },
          { selector: 'edge[edgeType="product_link"]', style: { width: 2, 'line-color': '#8b5cf6', 'target-arrow-color': '#8b5cf6' } },
          { selector: 'edge[edgeType="manifest_dep"]', style: { width: 3, 'line-color': '#3b82f6', 'target-arrow-color': '#3b82f6' } },
          { selector: 'edge[edgeType="ownership"]', style: { 'line-color': '#3b82f6', 'line-style': 'dashed', 'target-arrow-color': '#3b82f6' } },
          { selector: 'edge[edgeType="task_dep"]', style: { 'line-color': '#f59e0b', 'target-arrow-color': '#f59e0b' } },
        ],
        minZoom: 0.3, maxZoom: 3, wheelSensitivity: 0.3,
      }) as unknown as CyHandle;

      if (onNodeClick && cyInstance) {
        cyInstance.on('tap', 'node', (e) => {
          const d = e.target.data();
          onNodeClick(String(d.id ?? ''), String(d.type ?? ''));
        });
      }
    }).catch((err) => {
      console.error('cytoscape load failed', err);
    });

    return () => {
      destroyed = true;
      if (cyInstance) cyInstance.destroy();
    };
  }, [data, onNodeClick]);

  return <div ref={containerRef} className="product-dag-canvas" />;
}
