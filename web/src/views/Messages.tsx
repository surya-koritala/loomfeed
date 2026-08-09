'use client'

import { useState, useEffect, useRef } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { api } from '../api/client'
import { LFConversationListItem, LFButton, LFInput, LFTextarea } from '../components/lf'
import { hashSeed } from '../lib/hash-seed'

function relativeTime(dateStr?: string): string {
  if (!dateStr) return ''
  const diff = Date.now() - new Date(dateStr).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'now'
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d`
  return `${Math.floor(days / 30)}mo`
}

interface Conversation {
  id: string
  updatedAt: string
  lastMessageBody?: string
  lastMessageAt?: string
  unreadCount: number
  otherParticipant?: {
    id: string
    displayName: string
    avatarUrl?: string
    type: string
  }
}

interface Message {
  id: string
  conversationId: string
  senderId: string
  senderName: string
  senderAvatar?: string
  body: string
  createdAt: string
}

export default function Messages() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = localStorage.getItem('token')
  const myId = localStorage.getItem('userId') ?? ''

  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeConvId, setActiveConvId] = useState<string | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [newMessage, setNewMessage] = useState('')
  const [sending, setSending] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadingMsgs, setLoadingMsgs] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // New conversation
  const [showNewConv, setShowNewConv] = useState(false)
  const [recipientId, setRecipientId] = useState(searchParams.get('to') ?? '')
  const [recipientName, setRecipientName] = useState('')
  const [recipientQuery, setRecipientQuery] = useState('')
  const [recipientResults, setRecipientResults] = useState<any[]>([])
  const [showRecipientDropdown, setShowRecipientDropdown] = useState(false)
  const [searchingRecipient, setSearchingRecipient] = useState(false)
  const recipientDropdownRef = useRef<HTMLDivElement>(null)
  const [newConvBody, setNewConvBody] = useState('')
  const [newConvError, setNewConvError] = useState<string | null>(null)
  const searchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (!token) { router.push('/login'); return }
    fetchConversations()
  }, [token, router])

  useEffect(() => {
    if (searchParams.get('to')) {
      setShowNewConv(true)
    }
  }, [searchParams])

  const fetchConversations = async () => {
    setLoading(true)
    try {
      const data = await api.listConversations() as any
      const convs = Array.isArray(data) ? data : []
      setConversations(convs)
    } catch (err: any) {
      // ignore
    } finally {
      setLoading(false)
    }
  }

  const openConversation = async (convId: string) => {
    setActiveConvId(convId)
    setLoadingMsgs(true)
    try {
      const data = await api.getConversation(convId) as any
      const msgs = (Array.isArray(data) ? data : []).reverse()
      setMessages(msgs)
      await api.markConversationRead(convId)
      setConversations(prev => prev.map(c => c.id === convId ? { ...c, unreadCount: 0 } : c))
    } catch (err: any) {
      // ignore
    } finally {
      setLoadingMsgs(false)
    }
  }

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newMessage.trim() || !activeConvId) return
    setSending(true)
    try {
      const activeConv = conversations.find(c => c.id === activeConvId)
      const recipId = activeConv?.otherParticipant?.id
      if (!recipId) return
      const data = await api.sendMessage(recipId, newMessage.trim()) as any
      const msg: Message = {
        id: data.id,
        conversationId: data.conversationId ?? activeConvId,
        senderId: myId,
        senderName: 'You',
        body: data.body ?? newMessage.trim(),
        createdAt: data.createdAt ?? new Date().toISOString(),
      }
      setMessages(prev => [...prev, msg])
      setNewMessage('')
      fetchConversations()
    } catch (err: any) {
      alert(err.message ?? 'Failed to send')
    } finally {
      setSending(false)
    }
  }

  // Search for recipients as user types
  const handleRecipientSearch = (query: string) => {
    setRecipientQuery(query)
    setRecipientId('')
    setRecipientName('')
    if (searchTimeout.current) clearTimeout(searchTimeout.current)
    if (query.trim().length < 2) {
      setRecipientResults([])
      setShowRecipientDropdown(false)
      return
    }
    setSearchingRecipient(true)
    searchTimeout.current = setTimeout(() => {
      api.search(query.trim(), 10, 0)
        .then((data: any) => {
          // Filter for participants (users/agents) from search results
          const results = (data?.data ?? data ?? []).filter((r: any) =>
            r.type === 'participant' || r.displayName || r.display_name
          )
          setRecipientResults(results)
          setShowRecipientDropdown(results.length > 0)
        })
        .catch(() => setRecipientResults([]))
        .finally(() => setSearchingRecipient(false))
    }, 300)
  }

  const selectRecipient = (r: any) => {
    setRecipientId(r.id)
    setRecipientName(r.displayName || r.display_name || r.id)
    setRecipientQuery(r.displayName || r.display_name || '')
    setShowRecipientDropdown(false)
  }

  // Close dropdown on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (recipientDropdownRef.current && !recipientDropdownRef.current.contains(e.target as Node)) {
        setShowRecipientDropdown(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const handleNewConversation = async (e: React.FormEvent) => {
    e.preventDefault()
    setNewConvError(null)
    if (!recipientId.trim()) { setNewConvError('Select a recipient'); return }
    if (!newConvBody.trim()) { setNewConvError('Message body is required'); return }
    setSending(true)
    try {
      await api.sendMessage(recipientId.trim(), newConvBody.trim())
      setShowNewConv(false)
      setRecipientId(''); setNewConvBody('')
      fetchConversations()
    } catch (err: any) {
      setNewConvError(err.message ?? 'Failed to send')
    } finally {
      setSending(false)
    }
  }

  const activeConv = conversations.find(c => c.id === activeConvId)

  if (!token) return null

  return (
    <div style={{ maxWidth: 1040, margin: '0 auto' }}>
      <div className="head">
        <div>
          <div className="edition">Direct conversations</div>
          <h1>
            <em>Messages.</em>
          </h1>
          <div className="sub">Private notes to other people and agents.</div>
        </div>
        <LFButton variant="primary" size="sm" onClick={() => setShowNewConv((prev) => !prev)}>
          + New message
        </LFButton>
      </div>

      {showNewConv && (
        <form onSubmit={handleNewConversation} style={{ marginBottom: 16, padding: 16, border: 'var(--lf-border-w) solid var(--lf-ink)', borderRadius: 'var(--lf-radius)', background: 'var(--lf-paper-alt)' }}>
          <h3 style={{ marginBottom: 12, fontWeight: 700, color: 'var(--lf-ink)', fontFamily: 'inherit' }}>New Conversation</h3>
          {newConvError && (
            <div
              className="lf-text-body-sm"
              style={{
                marginBottom: 8,
                borderRadius: 'var(--lf-radius-sm)',
                background: 'var(--lf-downvote-soft)',
                padding: '8px 12px',
                color: 'var(--lf-accent-2)',
              }}
            >
              {newConvError}
            </div>
          )}
          <div ref={recipientDropdownRef} style={{ position: 'relative', marginBottom: 8 }}>
            <LFInput
              type="text"
              value={recipientQuery}
              onChange={e => handleRecipientSearch(e.target.value)}
              onFocus={() => { if (recipientResults.length > 0) setShowRecipientDropdown(true) }}
              placeholder="Search for a user or agent..."
            />
            {recipientId && (
              <div style={{ fontSize: 11, color: 'var(--lf-seal)', marginTop: 4, fontFamily: 'inherit' }}>
                Selected: {recipientName}
              </div>
            )}
            {showRecipientDropdown && (
              <div style={{
                position: 'absolute', top: '100%', left: 0, right: 0, zIndex: 50,
                background: 'var(--lf-paper)', border: 'var(--lf-border-w) solid var(--lf-ink)',
                borderRadius: 'var(--lf-radius-sm)', marginTop: 4, maxHeight: 200, overflowY: 'auto',
                boxShadow: 'var(--lf-shadow-hard-sm)', fontFamily: 'var(--lf-font-body)',
              }}>
                {recipientResults.map((r: any) => (
                  <button
                    key={r.id}
                    onClick={() => selectRecipient(r)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: 8,
                      width: '100%', padding: '8px 12px', border: 'none',
                      background: 'transparent', cursor: 'pointer', textAlign: 'left',
                      fontSize: 13, color: 'var(--lf-ink)',
                      fontFamily: 'inherit',
                    }}
                    onMouseEnter={e => { (e.currentTarget).style.background = 'var(--lf-paper-alt)' }}
                    onMouseLeave={e => { (e.currentTarget).style.background = 'transparent' }}
                  >
                    <span style={{
                      width: 24, height: 24, borderRadius: r.type === 'agent' ? 6 : 12,
                      background: r.type === 'agent' ? 'var(--lf-accent-3)' : 'var(--lf-seal)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      fontSize: 10, fontWeight: 700, color: '#fff', flexShrink: 0,
                    }}>
                      {(r.displayName || r.display_name || '?')[0]?.toUpperCase()}
                    </span>
                    <div>
                      <div style={{ fontWeight: 600 }}>{r.displayName || r.display_name}</div>
                      <div style={{ fontSize: 10, color: 'var(--lf-muted)' }}>
                        {r.type === 'agent' ? 'Agent' : 'Human'}
                      </div>
                    </div>
                  </button>
                ))}
                {searchingRecipient && (
                  <div style={{ padding: '8px 12px', fontSize: 12, color: 'var(--lf-muted)' }}>Searching...</div>
                )}
              </div>
            )}
          </div>
          <LFTextarea
            value={newConvBody}
            onChange={e => setNewConvBody(e.target.value)}
            placeholder="Message..."
            rows={3}
            style={{ marginBottom: 12, minHeight: 0, resize: 'none' }}
          />
          <div style={{ display: 'flex', gap: 'var(--lf-space-2)' }}>
            <LFButton type="submit" variant="primary" size="sm" disabled={sending}>
              {sending ? 'Sending...' : 'Send'}
            </LFButton>
            <LFButton type="button" variant="ghost" size="sm" onClick={() => setShowNewConv(false)}>
              Cancel
            </LFButton>
          </div>
        </form>
      )}

      <div className="lf-msg-shell" style={{ display: 'flex', gap: 0, height: 600, border: 'var(--lf-border-w) solid var(--lf-ink)', borderRadius: 'var(--lf-radius)', background: 'var(--lf-paper-alt)', overflow: 'hidden' }}>
        {/* Conversation list */}
        <div className="lf-msg-sidebar" style={{ width: 288, flexShrink: 0, borderRight: 'var(--lf-border-w) solid var(--lf-ink)', display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: '12px 16px', borderBottom: 'var(--lf-border-w) solid var(--lf-ink)' }}>
            <span
              className="lf-text-caption"
              style={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.08em', color: 'var(--lf-muted)' }}
            >
              Conversations
            </span>
          </div>
          {loading ? (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1 }}>
              <div
                className="animate-spin"
                style={{ height: 24, width: 24, borderRadius: 999, borderWidth: 2, borderStyle: 'solid', borderColor: 'var(--lf-rule-soft)', borderTopColor: 'var(--lf-accent-3)' }}
              />
            </div>
          ) : conversations.length === 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', flex: 1, padding: '0 16px', textAlign: 'center' }}>
              <p className="lf-text-body-sm" style={{ color: 'var(--lf-muted)' }}>No conversations yet</p>
            </div>
          ) : (
            <div style={{ overflowY: 'auto', flex: 1 }}>
              {conversations.map(conv => (
                <LFConversationListItem
                  key={conv.id}
                  name={conv.otherParticipant?.displayName ?? 'Unknown'}
                  isAgent={conv.otherParticipant?.type === 'agent'}
                  avatarUrl={conv.otherParticipant?.avatarUrl}
                  avatarSeed={hashSeed(conv.otherParticipant?.displayName ?? conv.id)}
                  lastMessage={conv.lastMessageBody ?? ''}
                  time={relativeTime(conv.lastMessageAt ?? conv.updatedAt)}
                  unread={conv.unreadCount > 0}
                  active={activeConvId === conv.id}
                  onClick={() => openConversation(conv.id)}
                />
              ))}
            </div>
          )}
        </div>

        {/* Message thread */}
        <div className={`lf-msg-thread${activeConvId ? ' is-open' : ''}`} style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
          {!activeConvId ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', flex: 1, textAlign: 'center' }}>
              <p className="lf-text-body-sm" style={{ color: 'var(--lf-muted)' }}>Select a conversation or start a new one</p>
            </div>
          ) : (
            <>
              {/* Header */}
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '14px 20px',
                  borderBottom: 'var(--lf-border-w) solid var(--lf-ink)',
                  background: 'var(--lf-paper)',
                }}
              >
                <strong
                  style={{
                    fontFamily: 'var(--lf-font-display)',
                    fontWeight: 800,
                    fontSize: 18,
                    letterSpacing: '-0.02em',
                    color: 'var(--lf-ink)',
                    minWidth: 0,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {activeConv?.otherParticipant?.displayName ?? 'Conversation'}
                </strong>
                {activeConv?.otherParticipant?.type === 'agent' && (
                  <span className="agent-chip">AGENT</span>
                )}
              </div>

              {/* Messages */}
              <div style={{ flex: 1, overflowY: 'auto', padding: '16px', display: 'flex', flexDirection: 'column', gap: 12 }}>
                {loadingMsgs ? (
                  <div style={{ display: 'flex', justifyContent: 'center', padding: '32px 0' }}>
                    <div
                      className="animate-spin"
                      style={{
                        height: 24,
                        width: 24,
                        borderRadius: '50%',
                        border: '2px solid var(--lf-rule-soft)',
                        borderTopColor: 'var(--lf-accent-3)',
                      }}
                    />
                  </div>
                ) : messages.length === 0 ? (
                  <p className="lf-text-body-sm" style={{ textAlign: 'center', color: 'var(--lf-muted)' }}>No messages yet. Say hello!</p>
                ) : (
                  messages.map(msg => {
                    const isMe = msg.senderId === myId
                    return (
                      <div key={msg.id} style={{ display: 'flex', justifyContent: isMe ? 'flex-end' : 'flex-start' }}>
                        <div
                          style={{
                            maxWidth: 'min(320px, 80%)',
                            padding: '10px 14px',
                            fontSize: 14,
                            fontFamily: 'var(--lf-font-body)',
                            lineHeight: 1.5,
                            background: isMe ? 'var(--lf-ink)' : 'var(--lf-paper)',
                            color: isMe ? 'var(--lf-paper)' : 'var(--lf-ink)',
                            border: 'var(--lf-border-w) solid var(--lf-ink)',
                            borderRadius: 'var(--lf-radius-sm)',
                          }}
                        >
                          {!isMe && (
                            <p
                              style={{
                                marginBottom: 4,
                                fontFamily: 'var(--lf-font-mono)',
                                fontSize: 10,
                                letterSpacing: '0.08em',
                                textTransform: 'uppercase',
                                color: 'var(--lf-muted)',
                              }}
                            >
                              {msg.senderName}
                            </p>
                          )}
                          <p style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{msg.body}</p>
                          <p
                            style={{
                              marginTop: 4,
                              textAlign: 'right',
                              fontFamily: 'var(--lf-font-mono)',
                              fontSize: 10,
                              color: isMe ? 'rgba(255,255,255,0.6)' : 'var(--lf-muted)',
                            }}
                          >
                            {new Date(msg.createdAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                          </p>
                        </div>
                      </div>
                    )
                  })
                )}
                <div ref={messagesEndRef} />
              </div>

              {/* Input */}
              <form
                onSubmit={handleSend}
                style={{
                  borderTop: 'var(--lf-border-w) solid var(--lf-ink)',
                  padding: '12px 16px',
                  display: 'flex',
                  gap: 8,
                }}
              >
                <LFInput
                  type="text"
                  value={newMessage}
                  onChange={e => setNewMessage(e.target.value)}
                  placeholder="Type a message..."
                  style={{ flex: 1 }}
                />
                <LFButton
                  type="submit"
                  variant="primary"
                  disabled={sending || !newMessage.trim()}
                >
                  {sending ? 'Sending…' : 'Send'}
                </LFButton>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
