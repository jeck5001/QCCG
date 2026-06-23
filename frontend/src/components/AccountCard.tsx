import { useEffect, useRef, useState } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GetAccountQuota } from '../../bindings/qccg/app'
import { isWebMode, getAccountQuota as apiGetAccountQuota } from '../api'

interface Account {
  id: string
  name: string
  email?: string
  auth_mode?: string
  api_mode?: string
  plan?: string
  active: boolean
}

interface QuotaBucket {
  used: number
  total: number
  remaining: number
  reset_time?: string
}

interface QuotaInfo {
  plan: string
  user_quota?: QuotaBucket
  org_resource_package?: QuotaBucket
  is_quota_exceeded?: boolean
  expires_at?: number
}

interface Props {
  account: Account
  onActivate: (id: string) => void
  onDelete: (id: string) => void
  refreshInterval?: number
}

// plan 名称关键词 → 配色
function planBadgeStyle(plan: string): { color: string; background: string } {
  const p = plan.toLowerCase()
  if (p.includes('pro')) return { color: '#7c3aed', background: 'rgba(124,58,237,0.1)' }
  if (p.includes('team') || p.includes('enterprise')) return { color: '#0369a1', background: 'rgba(3,105,161,0.1)' }
  if (p.includes('free') || p.includes('community')) return { color: '#6b7280', background: 'rgba(107,114,128,0.1)' }
  if (p.includes('trial')) return { color: '#d97706', background: 'rgba(217,119,6,0.1)' }
  return { color: '#7c3aed', background: 'rgba(124,58,237,0.1)' }
}

function timeAgo(ts: number | undefined): string {
  if (!ts) return ''
  const diff = Math.floor((Date.now() - ts) / 1000)
  if (diff < 60) return `${diff} 秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  return `${Math.floor(diff / 86400)} 天前`
}

function formatResetTime(ts: string | undefined): string {
  if (!ts) return ''
  const n = Number(ts)
  const d = isNaN(n) ? new Date(ts) : new Date(n)
  if (isNaN(d.getTime())) return ts
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

function countdown(expiresAtMs: number): { text: string; color: string } {
  const remaining = expiresAtMs - Date.now()
  const days = remaining / 86400000
  const d = Math.floor(days)
  const h = Math.floor((remaining % 86400000) / 3600000)
  const m = Math.floor((remaining % 3600000) / 60000)
  if (days > 365) return { text: '无限', color: 'var(--text-muted)' }
  if (remaining <= 0) return { text: '已到期', color: 'var(--danger)' }
  const text = `${d}d ${h}h ${m}m`

  // 渐变：30d muted → 7d warning → 1d danger
  const t = Math.max(0, Math.min(1, (30 - days) / 29))
  if (t < 0.5) return { text, color: `color-mix(in srgb, var(--text-muted) ${100 - t * 200}%, var(--warning) ${t * 200}%)` }
  return { text, color: `color-mix(in srgb, var(--warning) ${200 - t * 200}%, var(--danger) ${t * 200 - 100}%)` }
}

export default function AccountCard({ account: acct, onActivate, onDelete, refreshInterval = 300 }: Props) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: acct.id })
  const dragStyle = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : undefined,
  }

  const [quota, setQuota] = useState<QuotaInfo | null>(null)
  const [quotaError, setQuotaError] = useState(false)
  const [fetchedAt, setFetchedAt] = useState<number | undefined>()
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchQuota = () => {
    const promise = isWebMode() ? apiGetAccountQuota(acct.id) : GetAccountQuota(acct.id)
    promise
      .then((q: any) => {
        if (q) { setQuota(q); setFetchedAt(Date.now()); setQuotaError(false) }
      })
      .catch(() => setQuotaError(true))
  }

  useEffect(() => {
    setQuota(null); setQuotaError(false); setFetchedAt(undefined)
    fetchQuota()

    if (timerRef.current) clearInterval(timerRef.current)
    if (refreshInterval > 0) {
      timerRef.current = setInterval(fetchQuota, refreshInterval * 1000)
    }
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [acct.id, refreshInterval])

  // 到期倒计时，每分钟刷新
  const [remaining, setRemaining] = useState<{ text: string; color: string } | null>(null)
  useEffect(() => {
    const tick = () => {
      if (!quota?.expires_at) { setRemaining(null); return }
      setRemaining(countdown(quota.expires_at))
    }
    tick()
    countdownRef.current = setInterval(tick, 60000)
    return () => { if (countdownRef.current) clearInterval(countdownRef.current) }
  }, [quota?.expires_at])

  const plan = quota?.plan
  const displayName = acct.name || acct.email || acct.id
  const resetTime = quota?.user_quota?.reset_time

  return (
    <div ref={setNodeRef} style={dragStyle} {...attributes} {...listeners} className={`account-card ${acct.active ? 'active' : ''}`}>
      {/* 顶部：账号名 + badges */}
      <div className="ac-header">
        <div className="ac-name-row">
          <span className="ac-name">{displayName}</span>
          {quotaError && <span className="ac-badge ac-badge-warn">配额查询失败</span>}
          {quota?.is_quota_exceeded && <span className="ac-badge ac-badge-danger">配额已用尽</span>}
          {plan && <span className="ac-badge" style={planBadgeStyle(plan)}>{plan}</span>}
          <span className="ac-updated">{timeAgo(fetchedAt)}</span>
        </div>
        {(acct.email || acct.auth_mode) && (
          <div className="ac-meta">
            {[acct.email, acct.auth_mode?.toUpperCase()].filter(Boolean).join(' · ')}
          </div>
        )}
      </div>

      {/* 配额区块 */}
      <div className="ac-quota-section">
        <QuotaRow label="套餐内 Credits" countdown={remaining} bucket={quota?.user_quota} />
        <QuotaRow label="共享资源包" countdown={remaining} bucket={quota?.org_resource_package} />
        {resetTime && (
          <div className="ac-reset-time">订阅重置：{formatResetTime(resetTime)}</div>
        )}
      </div>

      {/* 底部：操作图标 */}
      <div className="ac-footer">
        <div className="ac-footer-actions">
          {!acct.active && (
            <button title="激活" className="ac-action-btn ac-action-activate" onClick={() => onActivate(acct.id)}>
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>
            </button>
          )}
          {acct.active && <span className="ac-active-badge">使用中</span>}
          <button title="刷新配额" className="ac-action-btn ac-action-refresh" onClick={fetchQuota}>
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
          </button>
          {!acct.active && (
          <button title="删除" className="ac-action-btn ac-action-danger" onClick={() => onDelete(acct.id)}>
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6M14 11v6"/><path d="M9 6V4h6v2"/></svg>
          </button>
          )}
        </div>
      </div>
    </div>
  )
}

function QuotaRow({ label, countdown, bucket }: { label: string; countdown?: { text: string; color: string } | null; bucket?: QuotaBucket }) {
  const used = bucket?.used ?? 0
  const total = bucket?.total ?? 0
  const usedPct = total > 0 ? Math.round((used / total) * 100) : 0
  const color = usedPct < 50 ? 'var(--success)' : usedPct < 80 ? 'var(--warning)' : 'var(--danger)'

  return (
    <div className="ac-quota-row">
      <div className="ac-quota-head">
        <span className="ac-quota-label">{label}</span>
        {countdown && <span className="ac-countdown" style={{ color: countdown.color }}>{countdown.text}</span>}
      </div>
      <div className="ac-quota-bar-bg">
        <div className="ac-quota-bar-fill" style={{ width: `${usedPct}%`, background: color }} />
      </div>
      <div className="ac-quota-stats">
        <span style={{ color }}>{usedPct}%</span>
        <span className="ac-quota-nums">{used.toFixed(0)} / {total.toFixed(0)}</span>
      </div>
    </div>
  )
}

function SharedRow({ value }: { value: number }) {
  return (
    <div className="ac-quota-row">
      <div className="ac-quota-label">共享资源包</div>
      <div className="ac-quota-bar-bg" />
      <div className="ac-quota-stats">
        <span className="ac-quota-nums">{value}</span>
      </div>
    </div>
  )
}
