import { lazy, Suspense, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useProductsByPeer, useProduct, useProductManifests, useProductHierarchy } from '../lib/queries/products';
import type { PeerProductGroup, Product } from '../lib/types';
import { TreeRow } from '../components/Tree';

const ProductDag = lazy(() => import('../components/ProductDag'));

export default function Products() {
  const { id: selectedId } = useParams();
  const navigate = useNavigate();
  const peerGroupsQuery = useProductsByPeer();

  const onSelect = (p: Product) => navigate(`/products/${p.id}`);

  return (
    <div className="page-products">
      <aside className="products-sidebar" data-testid="products-sidebar">
        <div className="sidebar-header">
          <h2>Products</h2>
          <button className="btn-new" type="button" onClick={() => alert('TODO: New product (T2)')}>
            + New Product
          </button>
        </div>
        {peerGroupsQuery.isLoading && <div className="empty-state">Loading…</div>}
        {peerGroupsQuery.error && <div className="empty-state error">Failed to load products</div>}
        {peerGroupsQuery.data && peerGroupsQuery.data.length === 0 && (
          <div className="empty-state">No products yet. Create one to group your manifests.</div>
        )}
        {peerGroupsQuery.data && peerGroupsQuery.data.map((pg: PeerProductGroup) => (
          <PeerGroup key={pg.peer_id} group={pg} selectedId={selectedId} onSelect={onSelect} />
        ))}
      </aside>
      <main className="products-detail">
        {!selectedId && <div className="empty-state">Select a product to view details</div>}
        {selectedId && <ProductDetail id={selectedId} />}
      </main>
    </div>
  );
}

function PeerGroup({ group, selectedId, onSelect }: {
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

function ProductTreeRow({ product, level, selectedId, onSelect }: {
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
          <span className={`status-dot status-${p.status}`} />
          <span className="product-title">{p.title}</span>
        </>
      )}
      extra={(p) => (
        <>
          <span className="marker">{p.marker}</span>
          {(p.total_manifests ?? 0) > 0 && <span className="count">{p.total_manifests}</span>}
          {(p.sub_products?.length ?? 0) > 0 && <span className="count count-sub">{p.sub_products!.length}</span>}
        </>
      )}
      children={() => subs}
      onSelect={(p) => onSelect(p)}
    />
  );
}

function ProductDetail({ id }: { id: string }) {
  const productQ = useProduct(id);
  const manifestsQ = useProductManifests(id);
  const [showDag, setShowDag] = useState(false);

  if (productQ.isLoading) return <div className="empty-state">Loading…</div>;
  if (productQ.error || !productQ.data) return <div className="empty-state error">Failed to load product</div>;
  const p = productQ.data;

  return (
    <div className="product-detail-body">
      <header className="product-header">
        <h1>{p.title}</h1>
        <div className="meta-row">
          <span className="marker">{p.marker}</span>
          <span className={`badge status-${p.status}`}>{p.status}</span>
          {(p.tags || []).map((t) => <span key={t} className="badge tag">{t}</span>)}
        </div>
        <div className="stats">
          <span>Manifests: <strong>{p.total_manifests ?? 0}</strong></span>
          <span>Tasks: <strong>{p.total_tasks ?? 0}</strong></span>
          <span>Turns: <strong>{p.total_turns ?? 0}</strong></span>
          <span>Cost: <strong>${(p.total_cost ?? 0).toFixed(2)}</strong></span>
        </div>
        <div className="toolbar">
          <button type="button" onClick={() => setShowDag((v) => !v)}>
            {showDag ? 'Hide DAG' : '◈ Product DAG'}
          </button>
        </div>
      </header>

      {p.description && <p className="product-description">{p.description}</p>}

      {showDag && (
        <Suspense fallback={<div className="empty-state">Loading diagram…</div>}>
          <ProductDagOverlay productId={id} onClose={() => setShowDag(false)} />
        </Suspense>
      )}

      <section className="linked-manifests">
        <h3>Linked Manifests {manifestsQ.data ? `(${manifestsQ.data.length})` : ''}</h3>
        {manifestsQ.isLoading && <div className="empty-state">Loading…</div>}
        {manifestsQ.data && manifestsQ.data.length === 0 && (
          <div className="empty-state">No manifests linked yet</div>
        )}
        {manifestsQ.data && manifestsQ.data.map((m) => (
          <div key={m.id} className="manifest-row">
            <span className="marker">{m.marker}</span>
            <span className={`badge status-${m.status}`}>{m.status}</span>
            <span className="manifest-title">{m.title}</span>
          </div>
        ))}
      </section>
    </div>
  );
}

function ProductDagOverlay({ productId, onClose }: { productId: string; onClose: () => void }) {
  const hierQ = useProductHierarchy(productId);
  return (
    <div className="dag-overlay">
      <div className="dag-overlay-header">
        <button type="button" onClick={onClose}>← Close</button>
        <span>Directed Acyclic Graph</span>
      </div>
      {hierQ.data && <ProductDag data={hierQ.data} />}
    </div>
  );
}
