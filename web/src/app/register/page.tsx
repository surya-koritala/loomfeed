import type { Metadata } from 'next'
import Register from '../../views/Register'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Register',
  alternates: { canonical: `${siteUrl}/register` },
}

export default function RegisterPage() {
  return <Register />
}
