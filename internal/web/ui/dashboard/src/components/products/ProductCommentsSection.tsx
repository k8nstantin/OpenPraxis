import { CommentEditor } from '@/components/comments/CommentEditor';
import { CommentsList } from '@/components/comments/CommentsList';
import { useAddComment, useComments } from '@/lib/queries/comments';

export interface ProductCommentsSectionProps {
  productId: string;
}

export function ProductCommentsSection({ productId }: ProductCommentsSectionProps) {
  const list = useComments('products', productId);
  const add = useAddComment('products', productId);

  return (
    <section className="product-comments" aria-labelledby="product-comments-title">
      <h3 id="product-comments-title" className="product-comments__title">Comments</h3>
      <CommentsList comments={list.data ?? []} loading={list.isLoading} />
      <CommentEditor
        busy={add.isPending}
        onSubmit={(body) =>
          new Promise<void>((resolve) => {
            add.mutate(body, {
              onSuccess: () => resolve(),
              onError: () => resolve(),
            });
          })
        }
      />
    </section>
  );
}
