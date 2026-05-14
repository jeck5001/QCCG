import { useState, useEffect } from 'react'

interface LogEntry {
  time: string
  level: string
  message: string
}

export default function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [filter, setFilter] = useState<string>('all')

  const fetchLogs = async () => {
    try {
      console.log('[LogsPage] Fetching logs...')
      const entries = await (window as any).go?.main?.App?.GetLogs(200)
      console.log('[LogsPage] Got entries:', entries)
      if (entries) {
        setLogs(entries)
      } else {
        console.log('[LogsPage] No entries returned')
      }
    } catch (e) {
      console.error('[LogsPage] Failed to fetch logs:', e)
    }
  }

  useEffect(() => {
    fetchLogs()
    if (autoRefresh) {
      const interval = setInterval(fetchLogs, 2000)
      return () => clearInterval(interval)
    }
  }, [autoRefresh])

  const handleClear = async () => {
    await (window as any).go?.main?.App?.ClearLogs()
    setLogs([])
  }

  const filteredLogs = logs.filter(log => {
    if (filter === 'all') return true
    return log.level === filter
  })

  const getLevelColor = (level: string) => {
    switch (level) {
      case 'debug': return 'var(--text-muted)'
      case 'info': return 'var(--primary)'
      case 'error': return 'var(--danger)'
      default: return 'var(--text-secondary)'
    }
  }

  return (
    <div className="logs-page">
      <div className="logs-header">
        <h2>日志</h2>
        <div className="logs-controls">
          <select value={filter} onChange={e => setFilter(e.target.value)}>
            <option value="all">全部</option>
            <option value="debug">Debug</option>
            <option value="info">Info</option>
            <option value="error">Error</option>
          </select>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={e => setAutoRefresh(e.target.checked)}
            />
            <span>自动刷新</span>
          </label>
          <button onClick={fetchLogs} className="btn btn-secondary">刷新</button>
          <button onClick={handleClear} className="btn btn-danger">清空</button>
        </div>
      </div>

      <div className="logs-container">
        {filteredLogs.length === 0 ? (
          <div className="logs-empty">暂无日志</div>
        ) : (
          filteredLogs.map((log, i) => (
            <div key={i} className="log-entry">
              <span className="log-time">{new Date(log.time).toLocaleTimeString()}</span>
              <span className="log-level" style={{ color: getLevelColor(log.level) }}>
                [{log.level.toUpperCase()}]
              </span>
              <span className="log-message">{log.message}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
