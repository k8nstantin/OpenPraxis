// Generic Cytoscape lifecycle wrapper. PR #240 hard-coded the DAG-specific
// element shape and stylesheet inside `ProductDag.tsx`; this component
// hoists the lifecycle (lazy module load, mount, destroy on unmount) so
// future tabs (manifest deps, task deps, peer mesh) reuse the same wiring.
import { useEffect, useRef } from 'react';

export interface CytoscapeElement {
  data: Record<string, unknown>;
  group?: 'nodes' | 'edges';
}

export interface CytoscapeStyle {
  selector: string;
  style: Record<string, unknown>;
}

export interface CytoscapeCanvasProps {
  elements: CytoscapeElement[];
  stylesheet: CytoscapeStyle[];
  layout?: Record<string, unknown>;
  onNodeClick?: (data: Record<string, unknown>) => void;
  className?: string;
  minZoom?: number;
  maxZoom?: number;
  wheelSensitivity?: number;
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
    // @ts-expect-error — runtime contract is small; declaring the full
    // cytoscape types would inflate node_modules-shipped d.ts coverage.
    cyModulePromise = import('cytoscape');
    // @ts-expect-error — same as above for cytoscape-dagre.
    dagreModulePromise = import('cytoscape-dagre');
  }
  const [cyMod, dagreMod] = await Promise.all([cyModulePromise!, dagreModulePromise!]);
  if (cyMod.default.use) cyMod.default.use(dagreMod.default);
  return cyMod.default;
}

export function CytoscapeCanvas({
  elements,
  stylesheet,
  layout,
  onNodeClick,
  className,
  minZoom = 0.3,
  maxZoom = 3,
  wheelSensitivity = 0.3,
}: CytoscapeCanvasProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let destroyed = false;
    let cyInstance: CyHandle | null = null;

    if (!containerRef.current) return;

    loadCytoscape()
      .then((cytoscape) => {
        if (destroyed || !containerRef.current) return;
        cyInstance = cytoscape({
          container: containerRef.current,
          elements,
          layout: layout ?? { name: 'dagre', rankDir: 'TB', nodeSep: 40, rankSep: 90, padding: 32, fit: true },
          style: stylesheet,
          minZoom,
          maxZoom,
          wheelSensitivity,
        }) as unknown as CyHandle;
        if (onNodeClick && cyInstance) {
          cyInstance.on('tap', 'node', (e) => onNodeClick(e.target.data()));
        }
      })
      .catch((err) => console.error('cytoscape load failed', err));

    return () => {
      destroyed = true;
      if (cyInstance) cyInstance.destroy();
    };
  }, [elements, stylesheet, layout, onNodeClick, minZoom, maxZoom, wheelSensitivity]);

  return <div ref={containerRef} className={className ?? 'cytoscape-canvas'} />;
}
