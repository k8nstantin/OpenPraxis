import { useQuery } from '@tanstack/react-query'

// Comment attachment surface (UB-2). The backend persists rows in
// comment_attachments + bytes under <data_dir>/attachments/<comment_id>/.
// Endpoints:
//   POST   /api/comments/{commentId}/attachments  (multipart/form-data)
//   GET    /api/comments/{commentId}/attachments
//   GET    /api/attachments/{id}
//   DELETE /api/attachments/{id}

export interface CommentAttachment {
  id: string
  comment_id: string
  filename: string
  mime_type: string
  size_bytes: number
  uploaded_by: string
  created_at: number
  url: string
}

export const attachmentKeys = {
  byComment: (commentId: string) => ['attachments', commentId] as const,
}

export function useCommentAttachments(commentId: string | undefined) {
  return useQuery({
    queryKey: attachmentKeys.byComment(commentId ?? ''),
    queryFn: async () => {
      const res = await fetch(`/api/comments/${commentId}/attachments`)
      if (!res.ok) throw new Error(`attachments → ${res.status}`)
      const data = (await res.json()) as
        | { attachments: CommentAttachment[] }
        | CommentAttachment[]
      return Array.isArray(data) ? data : (data.attachments ?? [])
    },
    enabled: !!commentId,
    staleTime: 30 * 1000,
  })
}

// uploadAttachment posts a single File via multipart. Returns the
// created attachment row, or throws with the backend's `code` field
// (e.g. "too_large" → 413, "mime_denied" → 415).
export async function uploadAttachment(
  commentId: string,
  file: File,
  author = 'operator'
): Promise<CommentAttachment> {
  const fd = new FormData()
  fd.append('file', file)
  fd.append('author', author)
  const res = await fetch(`/api/comments/${commentId}/attachments`, {
    method: 'POST',
    body: fd,
  })
  if (!res.ok) {
    let detail = ''
    try {
      const body = (await res.json()) as { error?: string; code?: string }
      detail = body.code ? `${body.code}: ${body.error ?? ''}` : (body.error ?? '')
    } catch {
      /* ignore */
    }
    throw new Error(detail || `upload failed (${res.status})`)
  }
  const data = (await res.json()) as { attachments: CommentAttachment[] }
  if (!data.attachments?.length) throw new Error('upload returned no rows')
  return data.attachments[0]
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}
