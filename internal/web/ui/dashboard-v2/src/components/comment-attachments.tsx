import { Download } from 'lucide-react'
import {
  formatBytes,
  useCommentAttachments,
} from '@/lib/queries/attachments'

// CommentAttachments renders the inline attachment block for a posted
// comment. Images render as <img> with lazy loading + a click-to-open
// link; everything else is a download chip.
export function CommentAttachments({ commentId }: { commentId: string }) {
  const q = useCommentAttachments(commentId)
  const items = q.data ?? []
  if (items.length === 0) return null

  return (
    <div className='mt-2 flex flex-wrap gap-2'>
      {items.map((a) => {
        const isImage = a.mime_type.startsWith('image/')
        if (isImage) {
          return (
            <a
              key={a.id}
              href={a.url}
              target='_blank'
              rel='noopener noreferrer'
              className='block max-w-[480px]'
              title={`${a.filename} — ${formatBytes(a.size_bytes)}`}
            >
              <img
                src={a.url}
                alt={a.filename}
                loading='lazy'
                className='border-border max-h-[480px] max-w-full rounded-sm border object-contain'
              />
            </a>
          )
        }
        return (
          <a
            key={a.id}
            href={a.url}
            target='_blank'
            rel='noopener noreferrer'
            className='border-border bg-muted/40 hover:bg-muted/60 flex items-center gap-2 rounded-sm border px-2 py-1 text-xs'
          >
            <Download className='h-3 w-3' />
            <span className='max-w-[16rem] truncate'>{a.filename}</span>
            <span className='text-muted-foreground'>
              {formatBytes(a.size_bytes)}
            </span>
          </a>
        )
      })}
    </div>
  )
}
