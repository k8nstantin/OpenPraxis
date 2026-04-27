import { lazy, Suspense, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  useProduct,
  useProductHierarchy,
  useProductsByPeer,
} from '@/lib/queries/products';
import type { PeerProductGroup, Product } from '@/lib/types';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { StatusDot } from '@/components/ui/StatusDot';
import { TreeRow } from '@/components/Tree';
import { MarkdownRenderer } from '@/components/desc/MarkdownRenderer';
import { LinkedIdeas } from '@/components/products/LinkedIdeas';
import { LinkedManifests } from '@/components/products/LinkedManifests';
import { NewProductDialog } from '@/components/products/NewProductDialog';
import { ProductCommentsSection } from '@/components/products/ProductCommentsSection';
import { ProductDeps } from '@/components/products/ProductDeps';
import { ProductEditForm } from '@/components/products/ProductEditForm';
import { ProductsSidebarSearch } from '@/components/products/ProductsSidebarSearch';
import { StatusPivots } from '@/components/products/StatusPivots';

const ProductDag = lazy(() => import('@/components/ProductDag'));

export default function Products() {
  const { id: selectedId } = useParams();
  const navigate = useNavigate();
  const peerGroupsQuery = useProductsByPeer();
  const [newProductOpen, setNewProductOpen] = useState(false);

  const onSelect = (p: Product) => navigate(`/products/${p.id}`);

  return (
    <div className="page-products">
      <aside className="products-sidebar" data-testid="products-sidebar">
        <div className="sidebar-header">
          <h2>Products</h2>
          <Button
            size="sm"
            variant="primary"
            data-testid="btn-new-product"
            onClick={() => setNewProductOpen(true)}
          >
            + New Product
          </Button>
        </div>
        <ProductsSidebarSearch />
        {peerGroupsQuery.isLoading && <EmptyState message="Loading…" />}
        {peerGroupsQuery.error && <EmptyState tone="error" message="Failed to load products" />}
        {peerGroupsQuery.data && peerGroupsQuery.data.length === 0 && (
          <EmptyState message="No products yet. Create one to group your manifests." />
        )}
        {peerGroupsQuery.data &&
          peerGroupsQuery.data.map((pg: PeerProductGroup) => (
            <PeerGroup key={pg.peer_id} group={pg} selectedId={selectedId} onSelect={onSelect} />
          ))}
        <NewProductDialog
          open={newProductOpen}
          onOpenChange={setNewProductOpen}
          onCreated={(p) => p?.id && navigate(`/products/${p.id}`)}
        />
      </aside>
      <main className="products-detail">
        {!selectedId && <EmptyState message="Select a product to view details" />}
        {selectedId && <ProductDetail id={selectedId} />}
      </main>
    </div>
  );
}

function PeerGroup({
  group,
  selectedId,
  onSelect,
}: {
  group: PeerProductGroup;
  selectedId: string | undefined;
  onSelect: (p: Product) => void;
}) {
  return (
    <TreeRow<PeerProductGroup>
      node={group}
      level={0}
      selected={false}
      initiallyExpanded
      rowKey={(g) => g.peer_id}
      label={(g) => <strong>{g.peer_id}</strong>}
      extra={(g) => <span className="count">{g.count}</span>}
      children={() => []}
      renderContent={(g) => (
        <div className="peer-group-products">
          {(g.products || []).map((p) => (
            <ProductTreeRow
              key={p.id}
              product={p}
              level={1}
              selectedId={selectedId}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}
    />
  );
}

function ProductTreeRow({
  product,
  level,
  selectedId,
  onSelect,
}: {
  product: Product;
  level: number;
  selectedId: string | undefined;
  onSelect: (p: Product) => void;
}) {
  const subs = product.sub_products || [];
  return (
    <TreeRow<Product>
      node={product}
      level={level}
      selected={product.id === selectedId}
      isSelected={(p) => p.id === selectedId}
      rowKey={(p) => p.id}
      label={(p) => (
        <>
          <StatusDot status={p.status} />
          <span className="product-title">{p.title}</span>
        </>
      )}
      extra={(p) => (
        <>
          <span className="marker">{p.marker}</span>
          {(p.total_manifests ?? 0) > 0 && <span className="count">{p.total_manifests}</span>}
          {(p.sub_products?.length ?? 0) > 0 && (
            <span className="count count-sub" title="Sub-products">{p.sub_products!.length}</span>
          )}
        </>
      )}
      children={() => subs}
      onSelect={(p) => onSelect(p)}
    />
  );
}

function ProductDetail({ id }: { id: string }) {
  const productQ = useProduct(id);
  const navigate = useNavigate();
  const [showDag, setShowDag] = useState(false);
  const [editing, setEditing] = useState(false);
  const [newSubOpen, setNewSubOpen] = useState(false);

  if (productQ.isLoading) return <EmptyState message="Loading…" />;
  if (productQ.error || !productQ.data) {
    return <EmptyState tone="error" message="Failed to load product" />;
  }
  const p = productQ.data;

  if (editing) {
    return (
      <div className="product-detail-body">
        <header className="product-header">
          <h1>Edit Product — {p.title}</h1>
        </header>
        <ProductEditForm
          product={p}
          onCancel={() => setEditing(false)}
          onSaved={() => setEditing(false)}
        />
      </div>
    );
  }

  return (
    <div className="product-detail-body">
      <header className="product-header">
        <nav className="breadcrumb" aria-label="Breadcrumb">
          <button
            type="button"
            className="breadcrumb-link"
            onClick={() => navigate('/products')}
          >
            {p.source_node ? p.source_node.substring(0, 12) : 'node'}
          </button>
          <span className="breadcrumb-sep"> → </span>
          <span>{p.marker} {p.title}</span>
        </nav>
        <h1>{p.title}</h1>
        <div className="meta-row">
          <span className="marker">{p.marker}</span>
          <Badge tone={p.status === 'open' ? 'success' : p.status === 'closed' ? 'neutral' : p.status === 'archive' ? 'danger' : 'warn'}>
            {p.status}
          </Badge>
          {(p.tags || []).map((t) => (
            <Badge key={t} tone="tag">{t}</Badge>
          ))}
        </div>
        <div className="stats">
          <span>Manifests<strong>{p.total_manifests ?? 0}</strong></span>
          <span>Tasks<strong>{p.total_tasks ?? 0}</strong></span>
          <span>Turns<strong>{p.total_turns ?? 0}</strong></span>
          <span>Cost<strong>${(p.total_cost ?? 0).toFixed(2)}</strong></span>
          {p.created_at && <span>Created<strong>{new Date(p.created_at).toLocaleDateString()}</strong></span>}
          {p.updated_at && <span>Updated<strong>{new Date(p.updated_at).toLocaleDateString()}</strong></span>}
        </div>
        <div className="toolbar product-toolbar">
          <Button size="sm" onClick={() => setEditing(true)}>✎ Edit</Button>
          <Button size="sm" onClick={() => setNewSubOpen(true)}>+ New Sub-Product</Button>
          <Button size="sm" onClick={() => setShowDag((v) => !v)}>
            {showDag ? 'Hide DAG' : '◈ Product DAG'}
          </Button>
          <StatusPivots productId={p.id} current={p.status} />
        </div>
      </header>

      {p.description && (
        <div className="product-description-wrap">
          <MarkdownRenderer source={p.description} html={p.description_html} className="product-description md-body" />
        </div>
      )}

      <ProductDeps product={p} onNavigate={(pid) => navigate(`/products/${pid}`)} />

      {showDag && (
        <Suspense fallback={<EmptyState message="Loading diagram…" />}>
          <ProductDagOverlay productId={id} onClose={() => setShowDag(false)} />
        </Suspense>
      )}

      <LinkedManifests productId={p.id} productTitle={p.title} />
      <LinkedIdeas productId={p.id} />
      <ProductCommentsSection productId={p.id} />

      <NewProductDialog
        open={newSubOpen}
        onOpenChange={setNewSubOpen}
        parent={{ id: p.id, title: p.title }}
        onCreated={(child) => child?.id && navigate(`/products/${child.id}`)}
      />
    </div>
  );
}

function ProductDagOverlay({ productId, onClose }: { productId: string; onClose: () => void }) {
  const hierQ = useProductHierarchy(productId);
  const navigate = useNavigate();
  return (
    <div className="dag-overlay">
      <div className="dag-overlay-header">
        <Button size="sm" onClick={onClose}>← Close</Button>
        <span>Directed Acyclic Graph</span>
      </div>
      {hierQ.data && (
        <ProductDag
          data={hierQ.data}
          onNodeClick={(nodeId, type) => {
            onClose();
            if (type === 'product') navigate(`/products/${nodeId}`);
            else if (type === 'manifest') navigate(`/manifests/${nodeId}`);
            else if (type === 'task') navigate(`/tasks/${nodeId}`);
          }}
        />
      )}
    </div>
  );
}
