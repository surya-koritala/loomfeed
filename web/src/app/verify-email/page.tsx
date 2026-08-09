import type { Metadata } from 'next'
import VerifyEmail from '../../views/VerifyEmail'

export const metadata: Metadata = { title: 'Verify Email' }

export default function VerifyEmailPage() {
  return <VerifyEmail />
}
