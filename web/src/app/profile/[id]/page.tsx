import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import Profile from '../../../views/Profile'
import { fetchApi } from '../../../lib/api-server'
import JsonLd from '../../../components/seo/JsonLd'

type Props = { params: Promise<{ id: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const profile = await fetchApi<any>(`/profiles/${id}`)
  if (!profile) notFound()
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'
  const name = profile.display_name || profile.displayName || 'Profile'
  const desc = (profile.bio || '').slice(0, 160) || `${name} on loomfeed`
  return {
    title: name,
    description: desc,
    alternates: {
      canonical: `${siteUrl}/profile/${id}`,
      types: {
        'application/rss+xml': `${siteUrl}/profile/${id}/feed.xml`,
      },
    },
    openGraph: { title: name, description: desc },
  }
}

export default async function ProfilePage({ params }: Props) {
  const { id } = await params
  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'
  // Fetch profile + posts in parallel — the posts query doesn't depend on
  // the profile result, so awaiting them serially cost two backend RTTs
  // before SSR could render.
  const [profile, postsResp] = await Promise.all([
    fetchApi<any>(`/profiles/${id}`),
    fetchApi<any>(`/profiles/${id}/posts?limit=25`),
  ])
  if (!profile) notFound()
  const posts: Array<{ id: string; title: string; body?: string; created_at?: string }> =
    (Array.isArray(postsResp) ? postsResp : postsResp?.data ?? postsResp?.posts ?? []) ?? []

  const name = profile?.display_name || profile?.displayName || ''
  const bio = profile?.bio || ''

  const jsonLd = profile
    ? {
        '@context': 'https://schema.org',
        '@type': 'Person',
        name,
        url: `${siteUrl}/profile/${id}`,
        description: bio.slice(0, 200),
      }
    : null

  return (
    <>
      {jsonLd && (
        <JsonLd data={jsonLd} />
      )}
      <Profile initialProfile={profile} initialPosts={posts} />
    </>
  )
}
