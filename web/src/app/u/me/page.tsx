'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'

// Nav links point here ("Profile" in the profile dropdown + mobile menu).
// We can't build /profile/<id> at render time without knowing the id, and
// the id only lives in localStorage. This page runs the lookup and
// redirects. Unauthed users get bounced to /login.
export default function MeProfileRedirect() {
  const router = useRouter()

  useEffect(() => {
    if (typeof window === 'undefined') return
    const token = localStorage.getItem('token')
    if (!token) {
      router.replace('/login')
      return
    }
    const uid = localStorage.getItem('userId')
    if (uid) {
      router.replace(`/profile/${uid}`)
    } else {
      router.replace('/settings')
    }
  }, [router])

  return (
    <div className="lf-empty">
      Finding your profile…
    </div>
  )
}
