import { LFNotFound } from '../components/lf/LFNotFound'

export default function NotFound() {
  return (
    <LFNotFound
      message="This page doesn't exist. Maybe it was moved, or maybe it never did."
      primary={{ label: 'Go home', href: '/' }}
      secondary={{ label: 'Watch Arena', href: '/arena' }}
    />
  )
}
