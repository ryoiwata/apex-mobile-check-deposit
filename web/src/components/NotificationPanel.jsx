import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

const TYPE_ICONS = {
  approved: '✅',
  rejected: '❌',
  returned: '↩️',
  completed: '🏦',
  funds_posted: '💵',
}

function formatTimeAgo(isoString) {
  const diffMs = Date.now() - new Date(isoString).getTime()
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  return `${Math.floor(diffHr / 24)}d ago`
}

/**
 * NotificationPanel shows investor notifications for a given account.
 * @param {{ accountId: string, onSelectTransfer: (transferId: string) => void, onUnreadChange: (count: number) => void }} props
 */
export default function NotificationPanel({ accountId, onSelectTransfer, onUnreadChange }) {
  const [notifications, setNotifications] = useState([])
  const [unreadCount, setUnreadCount] = useState(0)

  const fetchNotifications = useCallback(async () => {
    if (!accountId) return
    try {
      const data = await api.getNotifications(accountId)
      setNotifications(data.notifications || [])
      const count = data.unread_count || 0
      setUnreadCount(count)
      onUnreadChange?.(count)
    } catch {
      // Silent fail — stale notifications are acceptable
    }
  }, [accountId, onUnreadChange])

  useEffect(() => {
    fetchNotifications()
    const interval = setInterval(fetchNotifications, 5000)
    return () => clearInterval(interval)
  }, [fetchNotifications])

  async function handleClick(notif) {
    if (!notif.read) {
      try {
        await api.markNotificationRead(notif.id)
        setNotifications(prev => prev.map(n => n.id === notif.id ? { ...n, read: true } : n))
        const next = Math.max(0, unreadCount - 1)
        setUnreadCount(next)
        onUnreadChange?.(next)
      } catch {
        // Best-effort
      }
    }
    if (onSelectTransfer) {
      onSelectTransfer(notif.transfer_id)
    }
  }

  async function handleMarkAllRead() {
    try {
      await api.markAllNotificationsRead(accountId)
      setNotifications(prev => prev.map(n => ({ ...n, read: true })))
      setUnreadCount(0)
      onUnreadChange?.(0)
    } catch {
      // Best-effort
    }
  }

  if (!accountId || notifications.length === 0) return null

  return (
    <div style={{ marginBottom: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
        <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
          Notifications
          {unreadCount > 0 && (
            <span style={{
              marginLeft: 8,
              backgroundColor: '#dc2626',
              color: 'white',
              fontSize: 11,
              fontWeight: 700,
              borderRadius: 10,
              padding: '1px 7px',
            }}>
              {unreadCount}
            </span>
          )}
        </h3>
        {unreadCount > 0 && (
          <button
            onClick={handleMarkAllRead}
            style={{
              fontSize: 12,
              color: '#2563eb',
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              textDecoration: 'underline',
              padding: 0,
            }}
          >
            Mark all as read
          </button>
        )}
      </div>

      {notifications.map(notif => (
        <div
          key={notif.id}
          onClick={() => handleClick(notif)}
          style={{
            padding: '10px 14px',
            marginBottom: 6,
            borderRadius: 8,
            border: `1px solid ${notif.read ? '#e5e7eb' : '#bfdbfe'}`,
            backgroundColor: notif.read ? '#ffffff' : '#eff6ff',
            cursor: 'pointer',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 3 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span>{TYPE_ICONS[notif.type] || '📋'}</span>
              <strong style={{ fontSize: 13 }}>{notif.title}</strong>
              {!notif.read && (
                <span style={{
                  width: 7,
                  height: 7,
                  borderRadius: '50%',
                  backgroundColor: '#2563eb',
                  display: 'inline-block',
                  flexShrink: 0,
                }} />
              )}
            </div>
            <span style={{ fontSize: 11, color: '#6b7280', whiteSpace: 'nowrap', marginLeft: 8 }}>
              {formatTimeAgo(notif.created_at)}
            </span>
          </div>
          <p style={{ fontSize: 12, color: '#374151', margin: 0, lineHeight: 1.4 }}>
            {notif.message}
          </p>
        </div>
      ))}
    </div>
  )
}
