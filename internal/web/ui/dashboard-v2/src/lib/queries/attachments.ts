// Query keys for react-query cache invalidation
export const attachmentKeys = {
  byComment: (commentId: string) => ['attachments', 'comment', commentId] as const,
}

// Alias matching what ContentBlock/comments.tsx expect
export const uploadAttachment = async (file: File): Promise<string> => {
  const av = await uploadOrphanAttachment(file)
  return av.id
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

export type AttachmentView = {
  id: string
  comment_id: string
  filename: string
  mime_type: string
  size_bytes: number
  uploaded_by: string
  created_at: number
  url: string
}

async function readError(res: Response): Promise<string> {
  try {
    const j = (await res.json()) as { error?: string; code?: string }
    return j.error ?? `HTTP ${res.status}`
  } catch {
    return `HTTP ${res.status}`
  }
}

export async function uploadOrphanAttachment(
  file: File,
  author = 'operator'
): Promise<AttachmentView> {
  const fd = new FormData()
  fd.append('file', file)
  fd.append('author', author)
  const res = await fetch('/api/attachments', { method: 'POST', body: fd })
  if (!res.ok) throw new Error(await readError(res))
  const j = (await res.json()) as { attachments: AttachmentView[] }
  if (!j.attachments?.length) throw new Error('upload returned no attachment')
  return j.attachments[0]
}

export async function claimAttachment(
  attachmentId: string,
  commentId: string
): Promise<AttachmentView> {
  const res = await fetch(`/api/attachments/${attachmentId}/claim`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ comment_id: commentId }),
  })
  if (!res.ok) throw new Error(await readError(res))
  const j = (await res.json()) as { attachment: AttachmentView }
  return j.attachment
}

export async function listCommentAttachments(
  commentId: string
): Promise<AttachmentView[]> {
  const res = await fetch(`/api/comments/${commentId}/attachments`)
  if (!res.ok) throw new Error(await readError(res))
  const j = (await res.json()) as { attachments: AttachmentView[] }
  return j.attachments ?? []
}
