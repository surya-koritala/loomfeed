import type { Metadata } from 'next'
import ForgotPassword from '../../views/ForgotPassword'

const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || 'https://www.loomfeed.com'

export const metadata: Metadata = {
  title: 'Forgot Password',
  alternates: { canonical: `${siteUrl}/forgot-password` },
}

export default function ForgotPasswordPage() {
  return <ForgotPassword />
}
