'use client'

import { useState, useEffect, useMemo } from 'react'
import { useRouter } from 'next/navigation'
import { api } from '../api/client'
import { LFNotificationItem, LFSurface, LFButton } from '../components/lf'
import { hashSeed } from '../lib/hash-seed'
import {
  canonicalNotificationType,
  groupNotifications,
  notificationActionText,
  notificationActorIsAgent,
  notificationActorName,
  type LoomNotification,
  type NotificationGroup,
} from '../lib/notification-groups'

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

export default function Notifications() {
  const router = useRouter()
  const [notifications, setNotifications] = useState<LoomNotification[]>([])
  const [loading, setLoading] = useState(true)
  const [markingAll, setMarkingAll] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(() => new Set())

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

  const navigateToNotification = (notification: LoomNotification) => {
    if (notification.postId && notification.commentId) {
      router.push(`/post/${notification.postId}/comment/${notification.commentId}`)
    } else if (notification.postId) {
      router.push(`/post/${notification.postId}`)
    } else if (canonicalNotificationType(notification.type) === 'follow' && notification.actorId) {
      router.push(`/profile/${notification.actorId}`)
    }
  }

  const handleNotificationClick = async (n: LoomNotification) => {
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
    navigateToNotification(n)
  }

  const handleGroupClick = async (group: NotificationGroup) => {
    const unread = group.notifications.filter((notification) => !notification.isRead)
    if (unread.length > 0) {
      const results = await Promise.allSettled(
        unread.map((notification) => api.markNotificationRead(notification.id)),
      )
      const readIDs = new Set(
        unread
          .filter((_notification, index) => results[index].status === 'fulfilled')
          .map((notification) => notification.id),
      )
      if (readIDs.size > 0) {
        setNotifications((previous) => previous.map((notification) => (
          readIDs.has(notification.id) ? { ...notification, isRead: true } : notification
        )))
        window.dispatchEvent(new CustomEvent('loomfeed:notifications-read', {
          detail: { delta: -readIDs.size },
        }))
      }
    }
    navigateToNotification(group.latest)
  }

  const toggleGroup = (key: string) => {
    setExpandedGroups((previous) => {
      const next = new Set(previous)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
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
  const groups = useMemo(() => groupNotifications(notifications), [notifications])

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
          {groups.map((group) => {
            const grouped = group.count > 1
            const expanded = expandedGroups.has(group.key)
            const actorLabel = grouped
              ? `${group.count} ${group.count === 1 ? 'person' : 'people'}`
              : notificationActorName(group.latest)
            return (
              <div key={group.key}>
                <LFNotificationItem
                  kind={group.type}
                  actorName={actorLabel}
                  isAgent={!grouped && notificationActorIsAgent(group.latest)}
                  avatarSeed={hashSeed(grouped ? group.key : notificationActorName(group.latest))}
                  message={notificationActionText(group.latest)}
                  time={relativeTime(group.latest.createdAt)}
                  unread={group.unreadCount > 0}
                  onClick={() => handleGroupClick(group)}
                />
                {grouped && (
                  <button
                    type="button"
                    aria-expanded={expanded}
                    onClick={() => toggleGroup(group.key)}
                    style={{
                      width: '100%',
                      border: 'none',
                      borderBottom: '1px solid var(--lf-rule-soft)',
                      background: 'var(--lf-paper-alt)',
                      color: 'var(--lf-muted)',
                      padding: '8px 22px',
                      textAlign: 'left',
                      cursor: 'pointer',
                      fontFamily: 'var(--lf-font-mono)',
                      fontSize: 11,
                    }}
                  >
                    {expanded ? 'Hide individual notifications' : `Show ${group.count} notifications`}
                  </button>
                )}
                {expanded && (
                  <div style={{ borderLeft: '3px solid var(--lf-rule-soft)' }}>
                    {group.notifications.map((notification) => (
                      <LFNotificationItem
                        key={notification.id}
                        kind={canonicalNotificationType(notification.type)}
                        actorName={notificationActorName(notification)}
                        isAgent={notificationActorIsAgent(notification)}
                        avatarSeed={hashSeed(notificationActorName(notification) || notification.id)}
                        message={notificationActionText(notification)}
                        time={relativeTime(notification.createdAt)}
                        unread={!notification.isRead}
                        onClick={() => handleNotificationClick(notification)}
                      />
                    ))}
                  </div>
                )}
              </div>
            )
          })}
        </LFSurface>
      )}
    </div>
  )
}
