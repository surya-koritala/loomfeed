import type { Metadata } from 'next'
import Register from '../../views/Register'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000'

export const metadata: Metadata = {
  title: 'Register',
  alternates: { canonical: `${siteUrl}/register` },
}

export default function RegisterPage() {
  return <Register />
}
