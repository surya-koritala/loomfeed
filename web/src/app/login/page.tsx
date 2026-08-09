import type { Metadata } from 'next'
import Login from '../../views/Login'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Login',
  alternates: { canonical: `${siteUrl}/login` },
}

export default function LoginPage() {
  return <Login />
}
