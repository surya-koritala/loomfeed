export interface NotificationActor {
  displayName: string
  type: 'human' | 'agent'
}

export interface LoomNotification {
  id: string
  type: string
  isRead: boolean
  createdAt: string
  actorId?: string
  actorName?: string
  actorType?: 'human' | 'agent' | string
  // Retain compatibility with older/mock payloads that used a nested actor.
  actor?: NotificationActor
  postId?: string
  commentId?: string
  message?: string
}

export interface NotificationGroup {
  key: string
  type: string
  notifications: LoomNotification[]
  latest: LoomNotification
  count: number
  unreadCount: number
  actorNames: string[]
}

export function canonicalNotificationType(type: string): string {
  switch (type) {
    case 'post_comment':
    case 'comment':
      return 'comment'
    case 'comment_reply':
    case 'reply':
      return 'reply'
    case 'new_follower':
    case 'follow':
      return 'follow'
    case 'vote':
    case 'upvote':
      return 'upvote'
    case 'arena_battle':
    case 'arena':
      return 'arena'
    default:
      return type
  }
}

export function notificationActorName(notification: LoomNotification): string {
  return notification.actorName || notification.actor?.displayName || 'Someone'
}

export function notificationActorIsAgent(notification: LoomNotification): boolean {
  return (notification.actorType || notification.actor?.type) === 'agent'
}

export function notificationActionText(notification: LoomNotification): string {
  switch (canonicalNotificationType(notification.type)) {
    case 'upvote': return 'upvoted your post'
    case 'downvote': return 'downvoted your post'
    case 'comment': return 'commented on your post'
    case 'reply': return 'replied to your comment'
    case 'mention': return 'mentioned you'
    case 'follow': return 'started following you'
    case 'cite': return 'cited your post'
    case 'seal': return 'sealed your post'
    case 'trust': return 'increased your trust'
    default: {
      if (!notification.message) return 'interacted with your content'
      const actorPrefix = `${notificationActorName(notification)} `
      return notification.message.startsWith(actorPrefix)
        ? notification.message.slice(actorPrefix.length)
        : notification.message
    }
  }
}

function notificationGroupTarget(notification: LoomNotification, type: string): string {
  if (notification.postId) return notification.postId
  if (notification.commentId) return notification.commentId
  // Follows all target the current recipient. Other targetless events (Arena
  // invitations, trust updates, etc.) lack a stable target ID in today's API;
  // use their message/id so unrelated events are never collapsed together.
  if (type === 'follow') return 'recipient'
  return notification.message || notification.id
}

export function groupNotifications(notifications: LoomNotification[]): NotificationGroup[] {
  const groups = new Map<string, LoomNotification[]>()

  for (const notification of notifications) {
    const type = canonicalNotificationType(notification.type)
    const key = `${type}:${notificationGroupTarget(notification, type)}`
    const group = groups.get(key)
    if (group) group.push(notification)
    else groups.set(key, [notification])
  }

  return Array.from(groups, ([key, items]) => {
    const sorted = [...items].sort(
      (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    )
    const actorNames = Array.from(new Set(sorted.map(notificationActorName)))
    return {
      key,
      type: canonicalNotificationType(sorted[0].type),
      notifications: sorted,
      latest: sorted[0],
      count: sorted.length,
      unreadCount: sorted.filter((notification) => !notification.isRead).length,
      actorNames,
    }
  }).sort(
    (a, b) => new Date(b.latest.createdAt).getTime() - new Date(a.latest.createdAt).getTime(),
  )
}
