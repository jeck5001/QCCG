import { marked } from 'marked'
import DOMPurify from 'dompurify'

interface UpdateInfo {
  has_update: boolean
  current: string
  latest: string
  body: string
  download_url: string
  file_size: number
}

interface UpdateModalProps {
  updateInfo: UpdateInfo | null
  onDismiss: () => void
  onUpdate: () => void
  updating: boolean
  progress: number
  error: string | null
}

export default function UpdateModal({ updateInfo, onDismiss, onUpdate, updating, progress, error }: UpdateModalProps) {
  if (!updateInfo) return null

  const rawHtml = updateInfo.body
    ? DOMPurify.sanitize(marked.parse(updateInfo.body, { async: false }))
    : ''

  return (
    <div
      style={{
        position: 'fixed', inset: 0, zIndex: 1000,
        background: 'rgba(0,0,0,0.5)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}
      onClick={onDismiss}
    >
      <div
        style={{
          background: '#1e1e2e', borderRadius: 12, padding: '24px',
          width: 480, maxWidth: '90vw',
          boxShadow: '0 20px 60px rgba(0,0,0,0.5)',
          border: '1px solid rgba(255,255,255,0.08)',
        }}
        onClick={e => e.stopPropagation()}
      >
        <div style={{ marginBottom: 16 }}>
          <div style={{ fontSize: 18, fontWeight: 700, color: '#f97316' }}>
            发现新版本 {updateInfo.latest}
          </div>
          <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.4)', marginTop: 4 }}>
            当前版本：{updateInfo.current}
          </div>
        </div>

        <div style={{
          maxHeight: 300, overflowY: 'auto',
          background: 'rgba(255,255,255,0.04)',
          borderRadius: 8, padding: '12px 16px',
          marginBottom: 16,
          fontSize: 13, lineHeight: 1.6,
          color: 'rgba(255,255,255,0.75)',
        }}>
          {rawHtml ? (
            <div dangerouslySetInnerHTML={{ __html: rawHtml }} />
          ) : (
            <span style={{ color: 'rgba(255,255,255,0.3)', fontStyle: 'italic' }}>暂无更新说明</span>
          )}
        </div>

        {error && (
          <div style={{ color: '#ef4444', fontSize: 12, marginBottom: 12 }}>
            更新失败：{error}
          </div>
        )}

        {updating ? (
          <div>
            <div style={{
              height: 6, background: 'rgba(249,115,22,0.15)',
              borderRadius: 3, overflow: 'hidden', marginBottom: 8,
            }}>
              <div style={{
                height: '100%', background: '#f97316',
                borderRadius: 3, width: `${progress}%`,
                transition: 'width 0.3s ease', minWidth: 4,
              }} />
            </div>
            <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.4)', textAlign: 'center' }}>
              正在下载更新… {progress}%
            </div>
          </div>
        ) : (
          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
            <button
              onClick={onDismiss}
              style={{
                padding: '6px 16px', borderRadius: 6,
                border: '1px solid rgba(255,255,255,0.12)',
                background: 'transparent', color: 'rgba(255,255,255,0.6)',
                fontSize: 13, cursor: 'pointer',
              }}
            >
              关闭
            </button>
            <button
              onClick={onUpdate}
              style={{
                padding: '6px 16px', borderRadius: 6, border: 'none',
                background: '#f97316', color: '#fff',
                fontSize: 13, fontWeight: 600, cursor: 'pointer',
              }}
            >
              立即更新
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
