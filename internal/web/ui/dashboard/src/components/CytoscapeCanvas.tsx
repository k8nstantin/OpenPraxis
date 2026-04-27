import { useEffect, useRef } from 'react';

// Generic Cytoscape lifecycle wrapper. Mounts an instance against a
// ref'd <div>, destroys it on unmount, reinitialises whenever the
// elements/style/layout props change. cytoscape + cytoscape-dagre are
// loaded lazily by the consumer (via React.lazy on this component) so
// they don't bloat the initial bundle.

export interface CyHandle {
  destroy(): void;
  on(evt: string, sel: string, h: (e: { target: { data(): Record<string, unknown> } }) => void): void;
  fit?: () => void;
}

export interface CytoscapeFactory {
  (opts: unknown): CyHandle;
  use?(ext: unknown): void;
}

export interface CytoscapeCanvasProps {
  elements: unknown;
  style?: unknown;
  layout?: unknown;
  className?: string;
  minZoom?: number;
  maxZoom?: number;
  wheelSensitivity?: number;
  onNodeClick?: (id: string, type: string) => void;
}

let cyModulePromise: Promise<{ default: CytoscapeFactory }> | null = null;
let dagreModulePromise: Promise<{ default: unknown }> | null = null;

async function loadCytoscape(): Promise<CytoscapeFactory> {
  if (!cyModulePromise) {
    // @ts-expect-error — cytoscape ships its own types but we keep deps light.
    cyModulePromise = import('cytoscape');
    // @ts-expect-error — cytoscape-dagre.
    dagreModulePromise = import('cytoscape-dagre');
  }
  const [cyMod, dagreMod] = await Promise.all([cyModulePromise!, dagreModulePromise!]);
  if (cyMod.default.use) cyMod.default.use(dagreMod.default);
  return cyMod.default;
}

export default function CytoscapeCanvas({
  elements,
  style,
  layout,
  className = 'cy-canvas',
  minZoom = 0.3,
  maxZoom = 3,
  wheelSensitivity = 0.3,
  onNodeClick,
}: CytoscapeCanvasProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let destroyed = false;
    let cyInstance: CyHandle | null = null;
    if (!containerRef.current || !elements) return;

    loadCytoscape().then((cytoscape) => {
      if (destroyed || !containerRef.current) return;
      cyInstance = cytoscape({
        container: containerRef.current,
        elements,
        layout,
        style,
        minZoom,
        maxZoom,
        wheelSensitivity,
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
  }, [elements, style, layout, minZoom, maxZoom, wheelSensitivity, onNodeClick]);

  return <div ref={containerRef} className={className} />;
}
