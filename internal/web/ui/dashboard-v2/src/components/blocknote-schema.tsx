import {
  BlockNoteSchema,
  defaultBlockSpecs,
  defaultInlineContentSpecs,
} from '@blocknote/core'
import {
  createReactBlockSpec,
  createReactInlineContentSpec,
} from '@blocknote/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Layers, Package, ListChecks, AtSign } from 'lucide-react'
import type { EntityKind } from '@/lib/queries/entity'
import { entityHref, getEntityTitle } from '@/lib/queries/entity-search'

// OpenPraxis BlockNote schema — adds `mention` inline content + three
// custom block types (taskCard / manifestCard / productCard). Markdown
// round-trip degrades these to plain anchor tags via toExternalHTML;
// parse-back lands them as regular links (no auto-rehydration). The
// canonical source of truth is markdown; rich rendering only fires on
// fresh inserts inside the editor.

const KIND_ICON: Record<EntityKind, typeof Layers> = {
  product: Package,
  manifest: Layers,
  task: ListChecks,
}

const KIND_LABEL: Record<EntityKind, string> = {
  product: 'Product',
  manifest: 'Manifest',
  task: 'Task',
}

// --- mention inline content ---

export const Mention = createReactInlineContentSpec(
  {
    type: 'mention',
    propSchema: {
      kind: { default: 'product' },
      id: { default: '' },
      label: { default: '' },
    },
    content: 'none',
  },
  {
    render: (props) => {
      const kind = (props.inlineContent.props.kind ?? 'product') as EntityKind
      const id = props.inlineContent.props.id
      const Icon = KIND_ICON[kind] ?? AtSign
      const label =
        props.inlineContent.props.label || (id ? id.slice(0, 8) : '?')
      return (
        <Link
          to={entityHref({ kind, id, title: label })}
          className='border-border bg-muted/40 text-foreground hover:border-primary inline-flex items-center gap-1 rounded border px-1.5 py-0.5 align-baseline text-xs no-underline'
        >
          <Icon className='h-3 w-3' />
          <span>{label}</span>
        </Link>
      )
    },
    toExternalHTML: (props) => {
      const kind = (props.inlineContent.props.kind ?? 'product') as EntityKind
      const id = props.inlineContent.props.id
      const label = props.inlineContent.props.label || id.slice(0, 8)
      return (
        <a
          href={entityHref({ kind, id, title: label })}
          data-mention-kind={kind}
          data-mention-id={id}
        >
          {`@${label}`}
        </a>
      )
    },
  }
)

// --- entity card blocks ---

function EntityCard({ kind, id }: { kind: EntityKind; id: string }) {
  const q = useQuery({
    queryKey: ['blocknote-entity-card', kind, id],
    queryFn: () => getEntityTitle(kind, id),
    enabled: !!id,
    staleTime: 30_000,
  })
  const Icon = KIND_ICON[kind]
  const title = q.data ?? id.slice(0, 8)
  return (
    <Link
      to={entityHref({ kind, id, title })}
      className='border-border bg-card hover:border-primary my-1 block rounded-md border px-3 py-2 no-underline'
    >
      <div className='flex items-center gap-2'>
        <Icon className='text-muted-foreground h-4 w-4' />
        <span className='text-muted-foreground text-[10px] uppercase tracking-wider'>
          {KIND_LABEL[kind]}
        </span>
        <code className='text-muted-foreground font-mono text-[10px]'>
          {id.slice(0, 8)}
        </code>
      </div>
      <div className='text-foreground mt-1 text-sm font-medium'>{title}</div>
    </Link>
  )
}

function makeCardSpec(kind: EntityKind) {
  const type =
    kind === 'product'
      ? 'productCard'
      : kind === 'manifest'
        ? 'manifestCard'
        : 'taskCard'
  return createReactBlockSpec(
    {
      type,
      propSchema: {
        id: { default: '' },
      },
      content: 'none',
    },
    {
      render: (props) => {
        const id = props.block.props.id
        return (
          <div className='w-full' contentEditable={false}>
            {id ? (
              <EntityCard kind={kind} id={id} />
            ) : (
              <div className='border-border text-muted-foreground rounded-md border border-dashed px-3 py-2 text-xs italic'>
                {KIND_LABEL[kind]} card — no id set
              </div>
            )}
          </div>
        )
      },
      toExternalHTML: (props) => {
        const id = props.block.props.id
        if (!id) return <p />
        return (
          <a
            href={entityHref({ kind, id, title: id.slice(0, 8) })}
            data-block-type={type}
            data-id={id}
          >
            {`[${KIND_LABEL[kind]}: ${id.slice(0, 8)}]`}
          </a>
        )
      },
    }
  )
}

export const TaskCard = makeCardSpec('task')
export const ManifestCard = makeCardSpec('manifest')
export const ProductCard = makeCardSpec('product')

// --- assembled schema ---

export const opSchema = BlockNoteSchema.create({
  blockSpecs: {
    ...defaultBlockSpecs,
    taskCard: TaskCard(),
    manifestCard: ManifestCard(),
    productCard: ProductCard(),
  },
  inlineContentSpecs: {
    ...defaultInlineContentSpecs,
    mention: Mention,
  },
})

export type OpSchema = typeof opSchema
export type OpEditor = ReturnType<
  typeof opSchema.BlockNoteEditor
> extends infer T
  ? T
  : never
