import { buildDagElements } from '../lib/dag';
import type { ProductHierarchy } from '../lib/types';
import CytoscapeCanvas from './CytoscapeCanvas';

interface Props {
  data: ProductHierarchy | null | undefined;
  onNodeClick?: (id: string, type: string) => void;
}

const LAYOUT = {
  name: 'dagre',
  rankDir: 'TB',
  nodeSep: 40,
  rankSep: 90,
  edgeSep: 25,
  padding: 32,
  fit: true,
};

const STYLE = [
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
];

export default function ProductDag({ data, onNodeClick }: Props) {
  if (!data) return null;
  const elements = buildDagElements(data);
  return (
    <CytoscapeCanvas
      elements={elements}
      style={STYLE}
      layout={LAYOUT}
      className="product-dag-canvas"
      onNodeClick={onNodeClick}
    />
  );
}
