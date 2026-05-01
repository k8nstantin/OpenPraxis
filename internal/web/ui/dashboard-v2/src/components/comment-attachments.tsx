import { useQuery } from '@tanstack/react-query'
import {
  listCommentAttachments,
  type AttachmentView,
} from '@/lib/queries/attachments'
import { FileText, Download } from 'lucide-react'

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

// CommentAttachments — fetches + renders attachments for a posted
// comment. Images inline (lazy-loaded, capped width); everything else
// as a download chip.
export function CommentAttachments({ commentId }: { commentId: string }) {
  const q = useQuery({
    queryKey: ['attachments', commentId],
    queryFn: () => listCommentAttachments(commentId),
    staleTime: 30_000,
  })
  const list = q.data ?? []
  if (list.length === 0) return null
  return (
    <div className='mt-2 flex flex-wrap gap-2'>
      {list.map((a) => (
        <AttachmentTile key={a.id} a={a} />
      ))}
    </div>
  )
}

function AttachmentTile({ a }: { a: AttachmentView }) {
  if (a.mime_type.startsWith('image/')) {
    return (
      <a
        href={a.url}
        target='_blank'
        rel='noreferrer'
        className='border-border hover:border-primary block overflow-hidden rounded-md border'
      >
        <img
          src={a.url}
          alt={a.filename}
          loading='lazy'
          className='block max-h-60 max-w-[480px] object-contain'
        />
      </a>
    )
  }
  return (
    <a
      href={a.url}
      target='_blank'
      rel='noreferrer'
      download={a.filename}
      className='border-border bg-muted/40 hover:border-primary text-foreground inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs'
    >
      <FileText className='h-3.5 w-3.5' />
      <span className='font-mono'>{a.filename}</span>
      <span className='text-muted-foreground'>{fmtBytes(a.size_bytes)}</span>
      <Download className='text-muted-foreground h-3 w-3' />
    </a>
  )
}
