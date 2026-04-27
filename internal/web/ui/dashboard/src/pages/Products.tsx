// Products tab — full-parity React port of legacy `views/products.js`
// + `views/product-dag.js`. Sidebar peer-tree on the left; detail pane
// with header / toolbar / deps / linked-manifests / ideas / comments on
// the right; route param `/products/:id` selects a product so hard
// refresh keeps the operator's place.
import {
  Suspense,
  lazy,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
} from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  useAllIdeas,
  useAllManifests,
  useAddProductDep,
  useCreateManifest,
  useCreateProduct,
  useLinkIdeaToProduct,
  useLinkManifestToProduct,
  useProduct,
  useProductComments,
  useProductDeps,
  useProductHierarchy,
  useProductIdeas,
  useProductManifests,
  useProductSearch,
  useProductsByPeer,
  useRemoveProductDep,
  useUpdateProduct,
} from '../lib/queries/products';
import type {
  Idea,
  Manifest,
  PeerProductGroup,
  Product,
  ProductDep,
} from '../lib/types';
import { TreeRow } from '../components/Tree';
import { Button, Badge, Dialog, EmptyState } from '../components/ui';
import { FormField, FormActions } from '../components/forms';
import { CommentsList } from '../components/comments/CommentsList';
import { toast } from '../components/ui/Toast';
import { useResizable } from '../lib/hooks/useResizable';

const ProductDag = lazy(() => import('../components/ProductDag'));

const STATUS_OPTIONS = ['draft', 'open', 'closed', 'archive'] as const;
const TERMINAL_STATUSES = new Set(['closed', 'archive']);

export default function Products() {
  const { id: selectedId } = useParams();
  const navigate = useNavigate();
  const peerGroupsQuery = useProductsByPeer();
  const [searchTerm, setSearchTerm] = useState('');
  const [showNewProduct, setShowNewProduct] = useState(false);
  const debouncedSearch = useDebouncedValue(searchTerm.trim(), 200);
  const searchQuery = useProductSearch(debouncedSearch, debouncedSearch.length > 0);
  const { widthPx, onPointerDown } = useResizable({
    storageKey: 'openpraxis.productsSidebarWidth',
    defaultPx: 360,
    minPx: 220,
    maxPct: 0.6,
  });

  const onSelect = (p: Product) => navigate(`/products/${p.id}`);

  return (
    <div className="page-products" style={{ gridTemplateColumns: `${widthPx}px 6px 1fr` }}>
      <aside className="products-sidebar" data-testid="products-sidebar">
        <div className="sidebar-header">
          <h2>Products</h2>
          <Button size="sm" variant="primary" onClick={() => setShowNewProduct(true)}>
            + New Product
          </Button>
        </div>
        <input
          type="search"
          className="products-search"
          placeholder="Search products by id, marker, tag, or keyword..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          aria-label="Search products"
        />
        {searchTerm.trim().length > 0 ? (
          <SearchResults
            query={searchQuery.data}
            loading={searchQuery.isFetching}
            error={searchQuery.error}
            selectedId={selectedId}
            onSelect={onSelect}
          />
        ) : (
          <PeerTree
            data={peerGroupsQuery.data}
            loading={peerGroupsQuery.isLoading}
            error={peerGroupsQuery.error}
            selectedId={selectedId}
            onSelect={onSelect}
          />
        )}
      </aside>
      <div
        className="products-divider"
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize sidebar"
        onPointerDown={onPointerDown}
      />
      <main className="products-detail">
        {!selectedId && <EmptyState message="Select a product to view details." />}
        {selectedId && <ProductDetail id={selectedId} />}
      </main>
      <NewProductDialog
        open={showNewProduct}
        onOpenChange={setShowNewProduct}
        onCreated={(p) => {
          setShowNewProduct(false);
          navigate(`/products/${p.id}`);
        }}
      />
    </div>
  );
}

// ─────────────────────────── Sidebar pieces ───────────────────────────

function PeerTree({
  data, loading, error, selectedId, onSelect,
}: {
  data: PeerProductGroup[] | undefined;
  loading: boolean;
  error: unknown;
  selectedId: string | undefined;
  onSelect: (p: Product) => void;
}) {
  if (loading) return <EmptyState message="Loading…" />;
  if (error) {
    const msg = error instanceof Error ? error.message : String(error);
    return <EmptyState tone="error" message={`Failed to load products — ${msg}`} />;
  }
  if (!data || data.length === 0) {
    return <EmptyState message="No products yet. Create one to group your manifests." />;
  }
  return (
    <>
      {data.map((pg) => (
        <PeerGroup key={pg.peer_id} group={pg} selectedId={selectedId} onSelect={onSelect} />
      ))}
    </>
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
      label={(g) => <strong>{g.peer_id.slice(0, 12)}</strong>}
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

function SearchResults({
  query, loading, error, selectedId, onSelect,
}: {
  query: Product[] | undefined;
  loading: boolean;
  error: unknown;
  selectedId: string | undefined;
  onSelect: (p: Product) => void;
}) {
  if (loading) return <EmptyState message="Searching…" />;
  if (error) return <EmptyState message="Search failed" tone="error" />;
  if (!query || query.length === 0) return <EmptyState message="No products found" />;
  return (
    <ul className="search-results">
      {query.map((p) => (
        <li
          key={p.id}
          role="button"
          tabIndex={0}
          className={`search-result-row${p.id === selectedId ? ' is-selected' : ''}`}
          onClick={() => onSelect(p)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              onSelect(p);
            }
          }}
        >
          <div className="search-result-row__head">
            <span className={`status-dot status-${p.status}`} />
            <span className="marker">{p.marker}</span>
            <Badge tone={statusTone(p.status)}>{p.status}</Badge>
          </div>
          <div className="search-result-row__title">{p.title}</div>
        </li>
      ))}
    </ul>
  );
}

// ───────────────────────────── Detail pane ─────────────────────────────

function ProductDetail({ id }: { id: string }) {
  const productQ = useProduct(id);
  const [editing, setEditing] = useState(false);
  const [showDag, setShowDag] = useState(false);
  const [showLinkManifest, setShowLinkManifest] = useState(false);
  const [showLinkIdea, setShowLinkIdea] = useState(false);
  const [showAddDep, setShowAddDep] = useState(false);
  const [showNewManifest, setShowNewManifest] = useState(false);

  // Reset editing/overlay state on product switch.
  useEffect(() => {
    setEditing(false);
    setShowDag(false);
    setShowLinkManifest(false);
    setShowLinkIdea(false);
    setShowAddDep(false);
    setShowNewManifest(false);
  }, [id]);

  if (productQ.isLoading) return <EmptyState message="Loading…" />;
  if (productQ.error || !productQ.data) {
    const msg = productQ.error instanceof Error ? productQ.error.message : '';
    return <EmptyState tone="error" message={msg ? `Failed to load product — ${msg}` : 'Failed to load product'} />;
  }
  const p = productQ.data;

  return (
    <div className="product-detail-body">
      <Breadcrumb peer={p.source_node ?? 'node'} marker={p.marker} title={p.title} />
      {editing ? (
        <ProductEditForm
          product={p}
          onCancel={() => setEditing(false)}
          onSaved={() => setEditing(false)}
        />
      ) : (
        <>
          <ProductHeader product={p} />
          <ProductToolbar
            product={p}
            onEdit={() => setEditing(true)}
            onNewManifest={() => setShowNewManifest(true)}
            onLinkManifest={() => setShowLinkManifest(true)}
            onLinkIdea={() => setShowLinkIdea(true)}
            onToggleDep={() => setShowAddDep((v) => !v)}
            onShowDag={() => setShowDag(true)}
          />
          {p.description && (
            <div className="product-description">
              {/* DV/M5 trust passthrough is verbatim; markup mode is via DescToggle.
                  Render-only here matches the legacy descToggle's rendered branch. */}
              {p.description_html ? (
                <div dangerouslySetInnerHTML={{ __html: p.description_html }} />
              ) : (
                <pre className="product-description__raw">{p.description}</pre>
              )}
            </div>
          )}
          <ProductDeps
            productId={p.id}
            currentTitle={p.title}
            showAddPicker={showAddDep}
            onClosePicker={() => setShowAddDep(false)}
          />
          <ProductManifestsSection productId={p.id} />
          <ProductIdeasSection productId={p.id} />
          <ProductCommentsSection productId={p.id} />
        </>
      )}
      {showDag && (
        <Suspense fallback={<EmptyState message="Loading diagram…" />}>
          <ProductDagOverlay productId={id} title={p.title} onClose={() => setShowDag(false)} />
        </Suspense>
      )}
      {showLinkManifest && (
        <LinkManifestDialog
          productId={p.id}
          onClose={() => setShowLinkManifest(false)}
        />
      )}
      {showLinkIdea && (
        <LinkIdeaDialog
          productId={p.id}
          onClose={() => setShowLinkIdea(false)}
        />
      )}
      {showNewManifest && (
        <NewManifestDialog
          productId={p.id}
          onClose={() => setShowNewManifest(false)}
        />
      )}
    </div>
  );
}

function Breadcrumb({ peer, marker, title }: { peer: string; marker: string; title: string }) {
  return (
    <nav className="product-breadcrumb" aria-label="Breadcrumb">
      <ol>
        <li>{peer.slice(0, 12)}</li>
        <li className="product-breadcrumb__sep">→</li>
        <li>
          <span className="marker">{marker}</span> {title}
        </li>
      </ol>
    </nav>
  );
}

function ProductHeader({ product }: { product: Product }) {
  const tags = product.tags || [];
  const onCopy = () => {
    void navigator.clipboard.writeText(`get product ${product.marker}`);
    toast.success('Copied product reference');
  };
  return (
    <header className="product-header">
      <h1>{product.title}</h1>
      <div className="meta-row">
        <span className="marker">{product.marker}</span>
        <Badge tone={statusTone(product.status)} className={`status-${product.status}`}>{product.status}</Badge>
        {tags.map((t) => (
          <Badge key={t} className="tag" tone="info">
            {t}
          </Badge>
        ))}
        <button
          type="button"
          className="btn-copy"
          aria-label="Copy product reference"
          title="Copy product reference"
          onClick={onCopy}
        >
          ⎘
        </button>
      </div>
      <div className="stats">
        <span>Manifests <strong>{product.total_manifests ?? 0}</strong></span>
        <span>Tasks <strong>{product.total_tasks ?? 0}</strong></span>
        <span>Turns <strong>{product.total_turns ?? 0}</strong></span>
        <span>Cost <strong>${(product.total_cost ?? 0).toFixed(2)}</strong></span>
        {product.created_at && <span className="stats__time">Created {fmtDate(product.created_at)}</span>}
        {product.updated_at && <span className="stats__time">Updated {fmtDate(product.updated_at)}</span>}
      </div>
    </header>
  );
}

function ProductToolbar({
  product, onEdit, onNewManifest, onLinkManifest, onLinkIdea, onToggleDep, onShowDag,
}: {
  product: Product;
  onEdit: () => void;
  onNewManifest: () => void;
  onLinkManifest: () => void;
  onLinkIdea: () => void;
  onToggleDep: () => void;
  onShowDag: () => void;
}) {
  const update = useUpdateProduct();
  const onStatus = (s: string) => {
    if (s === product.status) return;
    update.mutate(
      { id: product.id, patch: { status: s } },
      { onError: (err) => toast.error(`Status change failed: ${err.message}`) },
    );
  };
  return (
    <div className="toolbar" role="toolbar" aria-label="Product actions">
      <Button size="sm" onClick={onEdit}>✎ Edit</Button>
      <Button size="sm" onClick={onNewManifest}>+ New Manifest</Button>
      <Button size="sm" variant="ghost" onClick={onLinkManifest}>+ Link Manifest</Button>
      <Button size="sm" variant="ghost" onClick={onToggleDep}>+ Depends On</Button>
      <Button size="sm" variant="ghost" onClick={onLinkIdea}>+ Link Idea</Button>
      <Button size="sm" variant="ghost" onClick={onShowDag}>◈ Product DAG</Button>
      <span className="toolbar__pivots" role="radiogroup" aria-label="Status">
        {STATUS_OPTIONS.map((s) => {
          const active = product.status === s;
          return (
            <button
              key={s}
              type="button"
              role="radio"
              aria-checked={active}
              data-testid={`status-pivot-${s}`}
              className={`status-pivot status-pivot--${s}${active ? ' is-active' : ''}`}
              onClick={() => onStatus(s)}
            >
              {s}
            </button>
          );
        })}
      </span>
    </div>
  );
}

// ─────────────────────────────── Edit form ──────────────────────────────

function ProductEditForm({
  product, onCancel, onSaved,
}: {
  product: Product;
  onCancel: () => void;
  onSaved: () => void;
}) {
  const update = useUpdateProduct();
  const [title, setTitle] = useState(product.title);
  const [description, setDescription] = useState(product.description ?? '');
  const [tags, setTags] = useState((product.tags ?? []).join(', '));
  const [error, setError] = useState<string | null>(null);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) {
      setError('Title is required');
      return;
    }
    update.mutate(
      {
        id: product.id,
        patch: {
          title: title.trim(),
          description,
          tags: tags.split(',').map((t) => t.trim()).filter(Boolean),
        },
      },
      {
        onSuccess: onSaved,
        onError: (err) => setError(err.message),
      },
    );
  };

  return (
    <form className="product-edit-form" onSubmit={onSubmit}>
      <h2 className="product-edit-form__title">Edit Product — {product.title}</h2>
      <FormField label="Title" htmlFor="edit-product-title" required error={error ?? undefined}>
        <input
          id="edit-product-title"
          className="form-input"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          autoFocus
        />
      </FormField>
      <FormField label="Description" htmlFor="edit-product-desc">
        <textarea
          id="edit-product-desc"
          className="form-input form-input--textarea"
          rows={10}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </FormField>
      <FormField label="Tags (comma-separated)" htmlFor="edit-product-tags">
        <input
          id="edit-product-tags"
          className="form-input"
          value={tags}
          onChange={(e) => setTags(e.target.value)}
        />
      </FormField>
      <FormActions>
        <Button type="button" variant="ghost" onClick={onCancel}>Cancel</Button>
        <Button type="submit" variant="primary" loading={update.isPending}>Save</Button>
      </FormActions>
    </form>
  );
}

// ──────────────────────────────── Deps ────────────────────────────────

function ProductDeps({
  productId, currentTitle, showAddPicker, onClosePicker,
}: {
  productId: string;
  currentTitle: string;
  showAddPicker: boolean;
  onClosePicker: () => void;
}) {
  const depsQ = useProductDeps(productId);
  const groupsQ = useProductsByPeer();
  const removeDep = useRemoveProductDep();
  const addDep = useAddProductDep();
  const navigate = useNavigate();

  const allProducts = useMemo<Product[]>(() => {
    const out: Product[] = [];
    for (const g of groupsQ.data ?? []) {
      for (const p of g.products ?? []) {
        out.push(p);
        for (const sub of p.sub_products ?? []) out.push(sub);
      }
    }
    return out;
  }, [groupsQ.data]);

  const deps = depsQ.data?.deps ?? [];
  const dependents = depsQ.data?.dependents ?? [];
  const depIds = new Set(deps.map((d) => d.id));
  const candidates = allProducts.filter(
    (cp) => cp.id !== productId && !depIds.has(cp.id) && cp.status !== 'archive',
  );

  const unsatisfied = deps.filter((d) => !TERMINAL_STATUSES.has(d.status));
  const satisfied = deps.length > 0 && unsatisfied.length === 0;

  return (
    <section className="product-deps" aria-labelledby="product-deps-title">
      <header className="product-deps__head">
        <h3 id="product-deps-title">Depends on</h3>
        {deps.length > 0 && (
          satisfied ? (
            <Badge tone="success" data-testid="deps-satisfied">✓ Satisfied</Badge>
          ) : (
            <Badge tone="warn" data-testid="deps-waiting" title={`Waiting on: ${unsatisfied.map((d) => d.marker).join(', ')}`}>
              ⏳ Waiting on {unsatisfied.length}
            </Badge>
          )
        )}
      </header>
      {deps.length === 0 && <p className="product-deps__empty">No dependencies</p>}
      {deps.length > 0 && (
        <ul className="product-deps__pills" aria-label="Dependencies (outgoing)">
          {deps.map((d) => (
            <DepPill
              key={d.id}
              dep={d}
              currentTitle={currentTitle}
              onNav={() => navigate(`/products/${d.id}`)}
              onRemove={() => {
                if (!window.confirm(`Remove dependency on ${d.title}?`)) return;
                removeDep.mutate({ productId, depId: d.id });
              }}
            />
          ))}
        </ul>
      )}
      {showAddPicker && (
        <DepPicker
          candidates={candidates}
          loading={groupsQ.isLoading}
          onSelect={(cid) => {
            addDep.mutate(
              { productId, dependsOnId: cid },
              { onSuccess: onClosePicker, onError: (err) => toast.error(err.message) },
            );
          }}
          onCancel={onClosePicker}
        />
      )}
      {dependents.length > 0 && (
        <div className="product-deps__incoming">
          <h4>Depended on by</h4>
          <ul className="product-deps__pills" aria-label="Dependencies (incoming)">
            {dependents.map((d) => (
              <DepPill key={d.id} dep={d} currentTitle={currentTitle} onNav={() => navigate(`/products/${d.id}`)} />
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

function DepPill({
  dep, onNav, onRemove,
}: {
  dep: ProductDep;
  currentTitle: string;
  onNav: () => void;
  onRemove?: () => void;
}) {
  return (
    <li className={`dep-pill dep-pill--${dep.status}`}>
      <button type="button" className="dep-pill__nav" onClick={onNav} title={`Open ${dep.title}`}>
        <span className="marker">{dep.marker}</span>
      </button>
      <span className="dep-pill__title">{dep.title}</span>
      <Badge tone={statusTone(dep.status)}>{dep.status}</Badge>
      {onRemove && (
        <button
          type="button"
          className="dep-pill__remove"
          aria-label={`Remove dependency on ${dep.title}`}
          onClick={onRemove}
        >
          ×
        </button>
      )}
    </li>
  );
}

function DepPicker({
  candidates, loading, onSelect, onCancel,
}: {
  candidates: Product[];
  loading: boolean;
  onSelect: (id: string) => void;
  onCancel: () => void;
}) {
  const [filter, setFilter] = useState('');
  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return candidates;
    return candidates.filter((c) =>
      c.title.toLowerCase().includes(q) || c.marker.toLowerCase().includes(q),
    );
  }, [candidates, filter]);
  return (
    <div className="dep-picker" role="dialog" aria-label="Add product dependency">
      <input
        type="search"
        className="form-input"
        placeholder="Search products to depend on…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        autoFocus
      />
      <ul className="dep-picker__list">
        {loading && <li className="dep-picker__empty">Loading…</li>}
        {!loading && filtered.length === 0 && <li className="dep-picker__empty">No matching products</li>}
        {filtered.map((c) => (
          <li key={c.id}>
            <button type="button" className="dep-picker__row" onClick={() => onSelect(c.id)}>
              <span className="marker">{c.marker}</span>
              <span className="dep-picker__title">{c.title}</span>
              <Badge tone={statusTone(c.status)}>{c.status}</Badge>
            </button>
          </li>
        ))}
      </ul>
      <FormActions>
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>Cancel</Button>
      </FormActions>
    </div>
  );
}

// ──────────────────────────── Manifests / Ideas ────────────────────────────

function ProductManifestsSection({ productId }: { productId: string }) {
  const q = useProductManifests(productId);
  const navigate = useNavigate();
  const unlink = useLinkManifestToProduct();
  return (
    <section className="linked-manifests" aria-labelledby="linked-manifests-title">
      <h3 id="linked-manifests-title">
        Linked Manifests {q.data ? <span className="sub-count">({q.data.length})</span> : ''}
      </h3>
      {q.isLoading && <EmptyState message="Loading…" />}
      {q.data && q.data.length === 0 && <EmptyState message="No manifests linked yet." />}
      {q.data && q.data.length > 0 && (
        <ul className="manifest-rows">
          {q.data.map((m) => (
            <li key={m.id} className="manifest-row">
              <button
                type="button"
                className="manifest-row__main"
                onClick={() => navigate(`/manifests/${m.id}`)}
                title={`Open manifest ${m.marker}`}
              >
                <span className="marker">{m.marker}</span>
                <Badge tone={statusTone(m.status)}>{m.status}</Badge>
                {(m.total_tasks ?? 0) > 0 && <span className="meta-strip">{m.total_tasks} tasks</span>}
                {(m.total_turns ?? 0) > 0 && <span className="meta-strip">{m.total_turns} turns</span>}
                {(m.total_cost ?? 0) > 0 && <span className="meta-strip cost">${(m.total_cost ?? 0).toFixed(2)}</span>}
                <span className="manifest-title">{m.title}</span>
              </button>
              <button
                type="button"
                className="row-remove"
                aria-label={`Remove ${m.title} from product`}
                onClick={() => {
                  if (!window.confirm(`Unlink ${m.title}?`)) return;
                  unlink.mutate({ manifestId: m.id, productId: null });
                }}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function ProductIdeasSection({ productId }: { productId: string }) {
  const q = useProductIdeas(productId);
  const unlink = useLinkIdeaToProduct();
  const ideas = q.data ?? [];
  return (
    <section className="linked-ideas" aria-labelledby="linked-ideas-title">
      <h3 id="linked-ideas-title">
        Linked Ideas {ideas.length > 0 ? <span className="sub-count">({ideas.length})</span> : ''}
      </h3>
      {q.isLoading && <EmptyState message="Loading…" />}
      {!q.isLoading && ideas.length === 0 && <EmptyState message="No ideas linked yet." />}
      {ideas.length > 0 && (
        <ul className="idea-rows">
          {ideas.map((i) => (
            <li key={i.id} className="idea-row">
              <span className="marker">{i.marker}</span>
              <Badge tone={statusTone(i.status)}>{i.status}</Badge>
              {i.priority && <span className={`priority priority--${i.priority}`}>{i.priority}</span>}
              <span className="idea-title">{i.title}</span>
              <button
                type="button"
                className="row-remove"
                aria-label={`Remove ${i.title} from product`}
                onClick={() => {
                  if (!window.confirm(`Unlink ${i.title}?`)) return;
                  unlink.mutate({ idea: i, productId: null });
                }}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function ProductCommentsSection({ productId }: { productId: string }) {
  const q = useProductComments(productId);
  return (
    <section className="product-comments" aria-labelledby="product-comments-title">
      <h3 id="product-comments-title">Comments</h3>
      <CommentsList comments={q.data ?? []} loading={q.isLoading} />
    </section>
  );
}

// ──────────────────────────── DAG overlay ────────────────────────────

function ProductDagOverlay({
  productId, title, onClose,
}: {
  productId: string;
  title: string;
  onClose: () => void;
}) {
  const hierQ = useProductHierarchy(productId);
  const navigate = useNavigate();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div className="dag-overlay" role="dialog" aria-modal="true" aria-label="Product DAG">
      <div className="dag-overlay-header">
        <button type="button" onClick={onClose}>← Back</button>
        <strong>{title}</strong>
        <span className="dag-overlay-header__sub">Directed Acyclic Graph</span>
        <span className="dag-overlay-header__legend">
          <span><span className="legend-dash legend-dash--ownership" /> hierarchy</span>
          <span><span className="legend-dash legend-dash--manifest" /> manifest dep</span>
          <span><span className="legend-dash legend-dash--task" /> task dep</span>
          <span>Scroll to zoom · Drag to pan · Click node to drill down</span>
        </span>
      </div>
      {hierQ.isLoading && <EmptyState message="Loading diagram…" />}
      {hierQ.error && <EmptyState tone="error" message="Failed to load diagram" />}
      {hierQ.data && (
        <ProductDag
          data={hierQ.data}
          onNodeClick={(id, type) => {
            onClose();
            if (type === 'product') navigate(`/products/${id}`);
            else if (type === 'manifest') window.location.assign(`/manifests?id=${id}`);
            else if (type === 'task') window.location.assign(`/tasks?id=${id}`);
          }}
        />
      )}
    </div>
  );
}

// ──────────────────────────── Dialogs ────────────────────────────

function NewProductDialog({
  open, onOpenChange, onCreated,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onCreated: (p: Product) => void;
}) {
  const create = useCreateProduct();
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [tags, setTags] = useState('');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setTitle('');
      setDescription('');
      setTags('');
      setError(null);
    }
  }, [open]);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) {
      setError('Title is required');
      return;
    }
    create.mutate(
      {
        title: title.trim(),
        description: description || undefined,
        tags: tags.split(',').map((t) => t.trim()).filter(Boolean),
      },
      {
        onSuccess: onCreated,
        onError: (err) => setError(err.message),
      },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="New Product"
      description="Group manifests under a shared product / project."
      size="md"
    >
      <form onSubmit={onSubmit}>
        <FormField label="Title" htmlFor="np-title" required error={error ?? undefined}>
          <input id="np-title" className="form-input" value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </FormField>
        <FormField label="Description" htmlFor="np-desc">
          <textarea id="np-desc" className="form-input form-input--textarea" rows={5} value={description} onChange={(e) => setDescription(e.target.value)} />
        </FormField>
        <FormField label="Tags (comma-separated)" htmlFor="np-tags">
          <input id="np-tags" className="form-input" value={tags} onChange={(e) => setTags(e.target.value)} />
        </FormField>
        <FormActions>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>Create Product</Button>
        </FormActions>
      </form>
    </Dialog>
  );
}

function NewManifestDialog({ productId, onClose }: { productId: string; onClose: () => void }) {
  const create = useCreateManifest();
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) {
      setError('Title is required');
      return;
    }
    create.mutate(
      { title: title.trim(), description, project_id: productId },
      { onSuccess: onClose, onError: (err) => setError(err.message) },
    );
  };
  return (
    <Dialog open onOpenChange={(v) => !v && onClose()} title="New Manifest" size="md">
      <form onSubmit={onSubmit}>
        <FormField label="Title" htmlFor="nm-title" required error={error ?? undefined}>
          <input id="nm-title" className="form-input" value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </FormField>
        <FormField label="Description" htmlFor="nm-desc">
          <textarea id="nm-desc" className="form-input form-input--textarea" rows={6} value={description} onChange={(e) => setDescription(e.target.value)} />
        </FormField>
        <FormActions>
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>Create Manifest</Button>
        </FormActions>
      </form>
    </Dialog>
  );
}

function LinkManifestDialog({ productId, onClose }: { productId: string; onClose: () => void }) {
  const all = useAllManifests(true);
  const link = useLinkManifestToProduct();
  const [filter, setFilter] = useState('');
  const candidates = useMemo<Manifest[]>(() => {
    const items = (all.data ?? []).filter((m) => m.project_id !== productId);
    const q = filter.trim().toLowerCase();
    if (!q) return items;
    return items.filter((m) => m.title.toLowerCase().includes(q) || m.marker.toLowerCase().includes(q));
  }, [all.data, filter, productId]);
  return (
    <Dialog open onOpenChange={(v) => !v && onClose()} title="Link Manifest to Product" size="lg">
      <input
        type="search"
        className="form-input"
        placeholder="Search by title or marker…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        autoFocus
      />
      {all.isLoading && <EmptyState message="Loading…" />}
      {!all.isLoading && candidates.length === 0 && <EmptyState message="No manifests available to link." />}
      <ul className="picker-list">
        {candidates.map((m) => (
          <li key={m.id}>
            <button
              type="button"
              className="picker-row"
              onClick={() =>
                link.mutate(
                  { manifestId: m.id, productId },
                  { onSuccess: onClose, onError: (err) => toast.error(err.message) },
                )
              }
            >
              <span className="marker">{m.marker}</span>
              <Badge tone={statusTone(m.status)}>{m.status}</Badge>
              <span className="picker-row__title">{m.title}</span>
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}

function LinkIdeaDialog({ productId, onClose }: { productId: string; onClose: () => void }) {
  const all = useAllIdeas(true);
  const link = useLinkIdeaToProduct();
  const [filter, setFilter] = useState('');
  const candidates = useMemo<Idea[]>(() => {
    const items = (all.data ?? []).filter((i) => i.project_id !== productId);
    const q = filter.trim().toLowerCase();
    if (!q) return items;
    return items.filter((i) => i.title.toLowerCase().includes(q) || i.marker.toLowerCase().includes(q));
  }, [all.data, filter, productId]);
  return (
    <Dialog open onOpenChange={(v) => !v && onClose()} title="Link Idea to Product" size="lg">
      <input
        type="search"
        className="form-input"
        placeholder="Search by title or marker…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        autoFocus
      />
      {all.isLoading && <EmptyState message="Loading…" />}
      {!all.isLoading && candidates.length === 0 && <EmptyState message="No ideas available to link." />}
      <ul className="picker-list">
        {candidates.map((i) => (
          <li key={i.id}>
            <button
              type="button"
              className="picker-row"
              onClick={() =>
                link.mutate(
                  { idea: i, productId },
                  { onSuccess: onClose, onError: (err) => toast.error(err.message) },
                )
              }
            >
              <span className="marker">{i.marker}</span>
              <Badge tone={statusTone(i.status)}>{i.status}</Badge>
              {i.priority && <span className={`priority priority--${i.priority}`}>{i.priority}</span>}
              <span className="picker-row__title">{i.title}</span>
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}

// ──────────────────────────── Helpers ────────────────────────────

function statusTone(status: string): 'info' | 'success' | 'warn' | 'danger' | undefined {
  switch (status) {
    case 'open':
    case 'completed':
      return 'success';
    case 'closed':
      return undefined;
    case 'archive':
    case 'failed':
      return 'danger';
    case 'draft':
    case 'running':
      return 'warn';
    default:
      return undefined;
  }
}

function fmtDate(s: string): string {
  try {
    return new Date(s).toLocaleString();
  } catch {
    return s;
  }
}

function useDebouncedValue<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setV(value), ms);
    return () => { if (timer.current) clearTimeout(timer.current); };
  }, [value, ms]);
  return v;
}

