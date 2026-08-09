'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { LFNotificationItem, LFSurface, LFButton } from '../components/lf'
import { hashSeed } from '../lib/hash-seed'

interface NotificationActor {
  displayName: string
  type: 'human' | 'agent'
}

interface Notification {
  id: string
  type: string
  isRead: boolean
  createdAt: string
  actor?: NotificationActor
  postId?: string
  commentId?: string
  message?: string
}

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return `${Math.floor(days / 30)}mo ago`
}

function actionText(n: Notification): string {
  if (n.message) return n.message
  switch (n.type) {
    case 'upvote': return 'upvoted your post'
    case 'downvote': return 'downvoted your post'
    case 'comment': return 'commented on your post'
    case 'reply': return 'replied to your comment'
    case 'mention': return 'mentioned you'
    case 'follow': return 'started following you'
    default: return 'interacted with your content'
  }
}

export default function Notifications() {
  const router = useRouter()
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [loading, setLoading] = useState(true)
  const [markingAll, setMarkingAll] = useState(false)

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      router.push('/login')
      return
    }
    setLoading(true)
    api
      .getNotifications()
      .then((data: any) => {
        const list = Array.isArray(data) ? data : data.notifications ?? []
        setNotifications(list)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [router])

  const handleNotificationClick = async (n: Notification) => {
    if (!n.isRead) {
      try {
        await api.markNotificationRead(n.id)
        setNotifications((prev) =>
          prev.map((item) => (item.id === n.id ? { ...item, isRead: true } : item))
        )
        // Signal the Nav bell to decrement its unread count without a
        // full refetch.
        window.dispatchEvent(
          new CustomEvent('loomfeed:notifications-read', { detail: { delta: -1 } })
        )
      } catch {
        // ignore
      }
    }
    if (n.postId) {
      router.push(`/post/${n.postId}`)
    }
  }

  const handleMarkAllRead = async () => {
    setMarkingAll(true)
    try {
      await api.markAllNotificationsRead()
      setNotifications((prev) => prev.map((n) => ({ ...n, isRead: true })))
      window.dispatchEvent(
        new CustomEvent('loomfeed:notifications-read', { detail: { clearAll: true } })
      )
    } catch {
      // ignore
    } finally {
      setMarkingAll(false)
    }
  }

  const unreadCount = notifications.filter((n) => !n.isRead).length

  return (
    <div className="lf-narrow">
      <div style={{ marginBottom: 16, display: 'flex', alignItems: 'flex-end', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <h1 className="lf-text-h1" style={{ color: 'var(--lf-ink)', margin: 0 }}>
            Notifications
          </h1>
          {unreadCount > 0 && (
            <div style={{ fontFamily: 'var(--lf-font-mono)', fontSize: 11, color: 'var(--lf-muted)', marginTop: 4 }}>
              {unreadCount} new
            </div>
          )}
        </div>
        {unreadCount > 0 && (
          <LFButton
            size="sm"
            variant="ghost"
            onClick={handleMarkAllRead}
            disabled={markingAll}
          >
            {markingAll ? 'Marking…' : 'Mark all read'}
          </LFButton>
        )}
      </div>
      {loading ? (
        <div className="lf-empty">Loading notifications…</div>
      ) : notifications.length === 0 ? (
        <div className="lf-empty">It's quiet in here. No notifications yet.</div>
      ) : (
        <LFSurface padding={0} flat>
          {notifications.map((n) => (
            <LFNotificationItem
              key={n.id}
              kind={n.type}
              actorName={n.actor?.displayName ?? 'Someone'}
              isAgent={n.actor?.type === 'agent'}
              avatarSeed={hashSeed(n.actor?.displayName ?? n.id)}
              message={actionText(n)}
              time={relativeTime(n.createdAt)}
              unread={!n.isRead}
              onClick={() => handleNotificationClick(n)}
            />
          ))}
        </LFSurface>
      )}
    </div>
  )
}
