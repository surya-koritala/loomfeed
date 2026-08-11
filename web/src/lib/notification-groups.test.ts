import { describe, expect, it } from 'vitest'
import {
  groupNotifications,
  notificationActionText,
  notificationActorName,
  type LoomNotification,
} from './notification-groups'

function notification(overrides: Partial<LoomNotification>): LoomNotification {
  return {
    id: 'notification-id',
    type: 'post_comment',
    isRead: false,
    createdAt: '2026-08-11T12:00:00Z',
    actorName: 'Alice',
    actorType: 'human',
    postId: 'post-1',
    commentId: 'comment-1',
    ...overrides,
  }
}

describe('groupNotifications', () => {
  it('groups notifications by canonical type and content target', () => {
    const groups = groupNotifications([
      notification({ id: 'newer', actorName: 'Alice', commentId: 'reply-2' }),
      notification({ id: 'older', actorName: 'Bob', commentId: 'reply-1', createdAt: '2026-08-11T11:00:00Z', isRead: true }),
    ])

    expect(groups).toHaveLength(1)
    expect(groups[0].key).toBe('comment:post-1')
    expect(groups[0].count).toBe(2)
    expect(groups[0].unreadCount).toBe(1)
    expect(groups[0].latest.id).toBe('newer')
    expect(groups[0].actorNames).toEqual(['Alice', 'Bob'])
  })

  it('keeps different types and targets in separate groups', () => {
    const groups = groupNotifications([
      notification({ id: 'comment-post-1' }),
      notification({ id: 'reply-post-1', type: 'comment_reply' }),
      notification({ id: 'comment-post-2', postId: 'post-2' }),
    ])

    expect(groups.map((group) => group.key)).toEqual([
      'comment:post-1',
      'reply:post-1',
      'comment:post-2',
    ])
  })

  it('does not collapse unrelated targetless Arena events', () => {
    const groups = groupNotifications([
      notification({ id: 'arena-1', type: 'arena_battle', postId: undefined, commentId: undefined, message: 'Arena: climate' }),
      notification({ id: 'arena-2', type: 'arena_battle', postId: undefined, commentId: undefined, message: 'Arena: energy' }),
    ])

    expect(groups).toHaveLength(2)
  })

  it('groups targetless follower notifications for the current recipient', () => {
    const groups = groupNotifications([
      notification({ id: 'follow-1', type: 'new_follower', postId: undefined, commentId: undefined }),
      notification({ id: 'follow-2', type: 'follow', postId: undefined, commentId: undefined }),
    ])

    expect(groups).toHaveLength(1)
    expect(groups[0].key).toBe('follow:recipient')
  })
})

describe('notification presentation', () => {
  it('uses the flat camel-cased actor fields returned by the API client', () => {
    expect(notificationActorName(notification({ actorName: 'Naomi', actor: undefined }))).toBe('Naomi')
  })

  it('produces action copy for current server notification types', () => {
    expect(notificationActionText(notification({ type: 'post_comment' }))).toBe('commented on your post')
    expect(notificationActionText(notification({ type: 'comment_reply' }))).toBe('replied to your comment')
    expect(notificationActionText(notification({ type: 'new_follower' }))).toBe('started following you')
    expect(notificationActionText(notification({ type: 'upvote' }))).toBe('upvoted your post')
  })
})
