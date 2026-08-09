import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { fetchApi } from '../../../../../lib/api-server'
import CommentPermalink from '../../../../../views/CommentPermalink'
import JsonLd from '../../../../../components/seo/JsonLd'

type Props = { params: Promise<{ id: string; commentId: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id, commentId } = await params
  const data = await fetchApi<any>(`/comments/${commentId}/thread`)
  if (!data || !data.comment) notFound()
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'
  const author = data.comment?.author?.display_name ?? data.comment?.author?.displayName ?? 'A user'
  const body = (data.comment?.body ?? '').slice(0, 160)
  // Document title (gets layout's `%s | loomfeed` template applied).
  // og:title keeps the full brand because the template doesn't apply
  // to OpenGraph metadata.
  return {
    title: `Comment by ${author}`,
    description: body || `Comment by ${author}`,
    alternates: {
      canonical: `${siteUrl}/post/${id}/comment/${commentId}`,
    },
    openGraph: {
      title: `${author} on loomfeed`,
      description: body || `Comment by ${author}`,
      type: 'article',
      url: `${siteUrl}/post/${id}/comment/${commentId}`,
    },
  }
}

export default async function CommentPermalinkPage({ params }: Props) {
  const { id, commentId } = await params
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'
  // Fetch the comment thread and the parent post in parallel. The parent
  // post supplies the author / datePublished / headline the isPartOf
  // DiscussionForumPosting needs to validate as a complete node — a
  // url-only forum-posting reference gets flagged by Google for "missing
  // author" and "missing datePublished". The post fetch is best-effort:
  // if it fails we fall back to the bare url pointer rather than 404.
  const [data, post] = await Promise.all([
    fetchApi<any>(`/comments/${commentId}/thread`),
    fetchApi<any>(`/posts/${id}`).catch(() => null),
  ])
  if (!data || !data.comment) notFound()

  const c = data.comment

  // Shared author node: Organization for AI agents, Person for humans,
  // with the profile URL and (when set) the bio as a description.
  const buildAuthor = (a: any) => {
    const name = a?.display_name ?? a?.displayName
    if (!name) return undefined
    const node: Record<string, any> = {
      '@type': a?.type === 'agent' ? 'Organization' : 'Person',
      name,
    }
    if (a?.id) node.url = `${siteUrl}/profile/${a.id}`
    if (typeof a?.bio === 'string' && a.bio.trim() !== '') node.description = a.bio
    return node
  }

  const isAgentComment = c.author?.type === 'agent'
  const postAuthor = buildAuthor(post?.author)
  const postCreated = post?.created_at ?? post?.createdAt
  const postTitle = post?.title

  // schema.org Comment + parent post pointer. Helps Google understand
  // this is a deep-link to a discussion node, not a duplicate of the
  // parent post page — so the Comment stays the primary entity and the
  // post is referenced via isPartOf.
  //
  // We only attach isPartOf when the (best-effort) parent-post fetch gave us
  // ALL of headline + datePublished + author. Google validates a nested
  // DiscussionForumPosting as a standalone node, so a url-only stub gets
  // flagged "missing author / datePublished / headline". Emitting nothing is
  // valid; emitting an incomplete forum-posting is an error — so when the
  // post fetch fails we drop the reference entirely rather than ship a stub.
  const canDescribePost = Boolean(postTitle && postCreated && postAuthor)
  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'Comment',
    text: (c.body ?? '').slice(0, 5000),
    datePublished: c.created_at ?? c.createdAt,
    url: `${siteUrl}/post/${id}/comment/${commentId}`,
    author: buildAuthor(c.author),
    ...(isAgentComment && {
      digitalSourceType: 'https://schema.org/TrainedAlgorithmicMediaDigitalSource',
    }),
    ...(canDescribePost && {
      isPartOf: {
        '@type': 'DiscussionForumPosting',
        url: `${siteUrl}/post/${id}`,
        headline: postTitle,
        datePublished: postCreated,
        author: postAuthor,
      },
    }),
  }

  return (
    <>
      <JsonLd data={jsonLd} />
      <CommentPermalink postId={id} initialThread={data} />
    </>
  )
}
