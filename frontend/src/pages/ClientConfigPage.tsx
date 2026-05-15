import { useState, useEffect, useMemo, useCallback, useRef } from 'react'
import { Wand2 as WandIcon, History, FolderSync, Rocket, Save, AlertTriangle } from 'lucide-react'
import ConfigEditor, { ConfigEditorHandle } from '../components/ConfigEditor'
import {
  GetClientConfigs,
  ListQoderModels,
  GetStatus,
  ReadClientConfigFile,
  SaveClientConfigFile,
  GetSettings,
  SaveSettings,
  HasClientConfigBackup,
  RestoreClientConfigFile,
} from '../../wailsjs/go/main/App'
import { account, main } from '../../wailsjs/go/models'
import claudeIcon from '../icons/clients/claude.svg'
import openaiIcon from '../icons/clients/openai.svg'
import geminiIcon from '../icons/clients/gemini.svg'

type ClientConfig = main.ClientConfig

const FALLBACK_QODER_KEYS = ['auto', 'ultimate', 'performance', 'efficient', 'lite']

const ICON_BY_TYPE: Record<string, string> = {
  claude: claudeIcon,
  codex: openaiIcon,
  gemini: geminiIcon,
}

// 默认映射「一键填充」内容，与后端 bridge.go::defaultModelMapping 对齐。
// 按 agent 分组，避免把 gpt 关键字塞进 claude 桶。
const DEFAULT_MAPPING_BY_AGENT: Record<string, Array<[string, string]>> = {
  claude: [['opus', 'ultimate'], ['sonnet', 'performance'], ['haiku', 'lite']],
  codex: [['gpt', 'performance']],
  gemini: [['gemini', 'performance']],
}

  // 客户端模型名候选，按 agent 提供建议
const CLIENT_MODEL_OPTIONS_BY_AGENT: Record<string, string[]> = {
  claude: ['sonnet', 'opus', 'haiku', 'claude-sonnet-4-6', 'claude-opus-4-7', 'claude-haiku-4-5', 'claude-sonnet-4-5', 'claude-3-5-sonnet-latest', 'claude-3-5-haiku-20241022'],
  codex: ['gpt', 'gpt-5', 'gpt-5-mini', 'gpt-5-nano', 'gpt-5-codex', 'gpt-4o', 'gpt-4o-mini', 'gpt-4.1', 'gpt-4.1-mini', 'o3', 'o3-mini', 'o4-mini', 'o1', 'o1-mini'],
  gemini: ['gemini', 'gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.5-flash-lite', 'gemini-2.0-flash', 'gemini-1.5-pro-latest', 'gemini-1.5-flash-latest'],
}

// ============== 配置文件文本工具 ==============
// 模型映射本质是 bridge 内部转换层，不应写入 CLI 配置文件本身。
// 但为了让用户在编辑器中直观看到映射变化：
//   - claude (JSON): 直接修改 env.ANTHROPIC_DEFAULT_OPUS/SONNET/HAIKU_MODEL 三个槽位
//   - codex/gemini: 追加注释行展示映射
// 保存到磁盘前会自动剥离注入内容，确保 CLI 文件保持干净。
// 注意：用户点击"保存"后，这三个槽位的值会真实写入磁盘（不剥离），
// 因为它们本来就是 Claude Code 读取的合法字段。

// claude JSON 中三个模型槽位字段与映射 from-key 的对应关系
const CLAUDE_MODEL_SLOTS: Array<{ envKey: string; fromKey: string }> = [
  { envKey: 'ANTHROPIC_DEFAULT_OPUS_MODEL',   fromKey: 'opus'   },
  { envKey: 'ANTHROPIC_DEFAULT_SONNET_MODEL', fromKey: 'sonnet' },
  { envKey: 'ANTHROPIC_DEFAULT_HAIKU_MODEL',  fromKey: 'haiku'  },
]

const PREVIEW_MAPPING_KEY = '_qoder2api_model_mapping'
const QODER_API_KEY = 'qoder2api'

interface MappingSelectProps {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
}

function MappingSelect({ value, onChange, options, placeholder = '请选择…' }: MappingSelectProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const selected = options.find(o => o.value === value)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div ref={ref} className="mapping-custom-select" onClick={() => setOpen(o => !o)}>
      <span className="mapping-custom-select-value">
        {selected ? selected.label : <span className="mapping-custom-select-placeholder">{placeholder}</span>}
      </span>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="12" height="12" style={{ flexShrink: 0, color: 'var(--text-muted)' }}>
        <polyline points="6 9 12 15 18 9"/>
      </svg>
      {open && (
        <div className="mapping-custom-dropdown">
          {options.map(o => (
            <div
              key={o.value}
              className={`mapping-custom-option${o.value === value ? ' selected' : ''}`}
              onMouseDown={e => { e.stopPropagation(); onChange(o.value); setOpen(false) }}
            >
              {o.label}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// 把 qoder2api 所需字段注入到配置文件内容（纯前端，不落盘）
function applyConfigToContent(content: string, format: string, agentType: string, port: number, apiKey: string): string {
  const baseURL = `http://127.0.0.1:${port}`
  if (format === 'json') {
    try {
      const obj = content.trim() ? JSON.parse(content) : {}
      if (!obj['env'] || typeof obj['env'] !== 'object') obj['env'] = {}
      const env = obj['env'] as Record<string, string>
      if (agentType === 'claude') {
        env['ANTHROPIC_BASE_URL'] = baseURL
        env['ANTHROPIC_AUTH_TOKEN'] = apiKey
      }
      return JSON.stringify(obj, null, 2) + '\n'
    } catch { return content }
  }
  if (format === 'toml') {
    try {
      const lines = content.trimEnd().split('\n')
      const filtered = lines.filter(l => !l.startsWith('model_provider'))
      filtered.push(`model_provider = "qoder2api"`)
      return filtered.join('\n') + '\n'
    } catch { return content }
  }
  if (format === 'dotenv') {
    const lines = content.trimEnd().split('\n').filter(l =>
      !l.startsWith('GOOGLE_GEMINI_BASE_URL=') && !l.startsWith('GEMINI_API_KEY=')
    )
    lines.push(`GOOGLE_GEMINI_BASE_URL=${baseURL}`)
    lines.push(`GEMINI_API_KEY=${apiKey}`)
    return lines.join('\n') + '\n'
  }
  return content
}

// 从配置文件内容中移除 qoder2api 注入的字段（纯前端，不落盘）
function removeConfigFromContent(content: string, format: string, agentType: string): string {
  if (format === 'json') {
    try {
      const obj = content.trim() ? JSON.parse(content) : {}
      if (obj['env'] && typeof obj['env'] === 'object') {
        const env = obj['env'] as Record<string, string>
        if (agentType === 'claude') {
          delete env['ANTHROPIC_BASE_URL']
          delete env['ANTHROPIC_AUTH_TOKEN']
          delete env['ANTHROPIC_MODEL']
          for (const { envKey } of CLAUDE_MODEL_SLOTS) delete env[envKey]
        }
        if (Object.keys(env).length === 0) delete obj['env']
      }
      return JSON.stringify(obj, null, 2) + '\n'
    } catch { return content }
  }
  if (format === 'toml') {
    return content.split('\n').filter(l =>
      !l.startsWith('model_provider')
    ).join('\n') + '\n'
  }
  if (format === 'dotenv') {
    return content.split('\n').filter(l =>
      !l.startsWith('GOOGLE_GEMINI_BASE_URL=') && !l.startsWith('GEMINI_API_KEY=')
    ).join('\n') + '\n'
  }
  return content
}

// qoder2API 修改的字段优先排在顶部，其余字段按字母序跟随
const TOP_LEVEL_KEY_ORDER = ['env', 'enabledPlugins', 'permissions', 'model', 'extensions', 'hooks']
const ENV_KEY_ORDER = [
  'ANTHROPIC_BASE_URL', 'ANTHROPIC_AUTH_TOKEN',
  'ANTHROPIC_DEFAULT_OPUS_MODEL', 'ANTHROPIC_DEFAULT_SONNET_MODEL', 'ANTHROPIC_DEFAULT_HAIKU_MODEL',
  'ANTHROPIC_MODEL',
]

function sortJSONKeys(obj: unknown): unknown {
  if (Array.isArray(obj)) return obj.map(sortJSONKeys)
  if (obj !== null && typeof obj === 'object') {
    const o = obj as Record<string, unknown>
    const sortWithPriority = (keys: string[], priorityList: string[]) => {
      const priority = keys.filter(k => priorityList.includes(k)).sort((a, b) => priorityList.indexOf(a) - priorityList.indexOf(b))
      const rest = keys.filter(k => !priorityList.includes(k)).sort()
      return [...priority, ...rest]
    }
    // 顶层用 TOP_LEVEL_KEY_ORDER，env 子对象用 ENV_KEY_ORDER
    const isEnvObj = Object.keys(o).some(k => ENV_KEY_ORDER.includes(k))
    const ordered = sortWithPriority(Object.keys(o), isEnvObj ? ENV_KEY_ORDER : TOP_LEVEL_KEY_ORDER)
    const result: Record<string, unknown> = {}
    for (const k of ordered) result[k] = sortJSONKeys(o[k])
    return result
  }
  return obj
}

// 仅 JSON 支持"整理格式"。TOML / dotenv 含注释，parse-stringify 会丢失语义，参考 cc-switch 同样禁用。
function formatContent(content: string, format: string): string {
  if (format !== 'json') throw new Error(`${format} 不支持自动整理（会丢失注释）`)
  const trimmed = content.trim()
  if (!trimmed) return ''
  return JSON.stringify(sortJSONKeys(JSON.parse(trimmed)), null, 2) + '\n'
}

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// 从预览文本中剥离 qoder2api 注入的映射展示字段，得到"干净"的 CLI 文件原文
// claude JSON: 三个 DEFAULT_*_MODEL 槽位是合法字段，不剥离（用户保存即真实写入）
function stripMappingPreview(content: string, format: string): string {
  if (!content) return content
  if (format === 'json') {
    try {
      const obj = JSON.parse(content)
      if (obj && typeof obj === 'object' && PREVIEW_MAPPING_KEY in obj) {
        delete obj[PREVIEW_MAPPING_KEY]
        return JSON.stringify(obj, null, 2) + '\n'
      }
    } catch {
      // 文本不是合法 JSON（用户正在编辑），保持原样
    }
    return content
  }
  if (format === 'toml') {
    // 移除 [_qoder2api_model_mapping] 段（含其后续 KV 行，直到下一段或文件结束）
    const re = new RegExp(`(?:^|\\n)\\[${escapeRe(PREVIEW_MAPPING_KEY)}\\][^\\n]*(?:\\n(?!\\[)[^\\n]*)*`, 'g')
    return content.replace(re, '').replace(/\n{3,}/g, '\n\n').replace(/\s+$/, '') + '\n'
  }
  if (format === 'dotenv') {
    const re = new RegExp(`(?:^|\\n)# ${escapeRe(PREVIEW_MAPPING_KEY)}=[^\\n]*`, 'g')
    return content.replace(re, '').replace(/\n{3,}/g, '\n\n').replace(/\s+$/, '') + '\n'
  }
  return content
}

// 把当前 agent 的映射注入到预览文本（先 strip 旧的，再插入新的，保证幂等）
// claude JSON: 把映射结果直接写入 env 中三个 DEFAULT_*_MODEL 槽位（合法字段，保存时不剥离）
function patchMappingPreview(content: string, format: string, mapping: Record<string, string> | undefined, agent?: string): string {
  const cleaned = stripMappingPreview(content, format)
  if (format === 'json' && agent === 'claude') {
    // 无论 mapping 是否为空，都先清槽位再按需写入，保证删行后槽位消失
    try {
      const obj = cleaned.trim() ? JSON.parse(cleaned) : {}
      if (obj['env'] && typeof obj['env'] === 'object') {
        const env = obj['env'] as Record<string, string>
        for (const { envKey } of CLAUDE_MODEL_SLOTS) delete env[envKey]
        if (Object.keys(env).length === 0) delete obj['env']
      }
      if (mapping && Object.keys(mapping).length > 0) {
        if (!obj['env'] || typeof obj['env'] !== 'object') obj['env'] = {}
        const env = obj['env'] as Record<string, string>
        for (const { envKey, fromKey } of CLAUDE_MODEL_SLOTS) {
          const mappedTo = mapping[fromKey]
          if (mappedTo) env[envKey] = mappedTo
        }
      }
      return JSON.stringify(obj, null, 2) + '\n'
    } catch {
      return content
    }
  }
  if (!mapping || Object.keys(mapping).length === 0) return cleaned
  if (format === 'json') {
    try {
      const obj = cleaned.trim() ? JSON.parse(cleaned) : {}
      obj[PREVIEW_MAPPING_KEY] = mapping
      return JSON.stringify(obj, null, 2) + '\n'
    } catch {
      // 用户正在编辑、JSON 暂时损坏 → 不强行注入
      return content
    }
  }
  if (format === 'toml') {
    const head = cleaned.replace(/\s+$/, '')
    const lines: string[] = []
    lines.push(`[${PREVIEW_MAPPING_KEY}]  # qoder2api 模型映射预览（仅展示，由 Settings.json 管理；保存时不会写入此文件）`)
    for (const [k, v] of Object.entries(mapping)) {
      const key = /^[A-Za-z0-9_-]+$/.test(k) ? k : JSON.stringify(k)
      lines.push(`${key} = ${JSON.stringify(v)}`)
    }
    return (head ? head + '\n\n' : '') + lines.join('\n') + '\n'
  }
  if (format === 'dotenv') {
    const head = cleaned.replace(/\s+$/, '')
    return (head ? head + '\n' : '') + `# ${PREVIEW_MAPPING_KEY}=${JSON.stringify(mapping)}\n`
  }
  return cleaned
}

export default function ClientConfigPage() {
  const [configs, setConfigs] = useState<ClientConfig[]>([])
  const [qoderModels, setQoderModels] = useState<main.QoderModel[]>([])
  const [loading, setLoading] = useState(true)
  const [applying, _setApplying] = useState<string | null>(null)
  const [status, setStatus] = useState<{ running: boolean; port: number }>({ running: false, port: 8963 })
  const [activeType, setActiveType] = useState<string>('claude')

  // 配置文件编辑器
  const [fileLoading, setFileLoading] = useState(false)
  const [fileSaving, setFileSaving] = useState(false)
  const [fileContent, setFileContent] = useState('')
  const [fileMeta, setFileMeta] = useState<{ path: string; format: string; existed: boolean } | null>(null)
  const [fileDirty, setFileDirty] = useState(false)
  const [fileError, setFileError] = useState<string | null>(null)
  const [formatStatus, setFormatStatus] = useState<'idle' | 'done' | 'error'>('idle')
  const [hasBackup, setHasBackup] = useState(false)
  const editorRef = useRef<ConfigEditorHandle>(null)

  // 客户端模型名 → Qoder model.key 的待保存映射（按 agent 暂存到内存，仅在用户点击"保存"时写后端）
  const [pendingMapping, setPendingMapping] = useState<Record<string, Record<string, string>> | null>(null)
  const [mappingDirty, setMappingDirty] = useState(false)

  const refresh = async () => {
    try {
      const [c, m, s] = await Promise.all([
        GetClientConfigs(),
        ListQoderModels().catch(() => [] as main.QoderModel[]),
        GetStatus().catch(() => ({ running: false, port: 8963 })),
      ])
      setConfigs(c || [])
      setQoderModels(m || [])
      if (s) setStatus(s as any)
    } catch (err) {
      console.error('Failed to load configs:', err)
    } finally {
      setLoading(false)
    }
  }

  const [fileLoadKey, setFileLoadKey] = useState(0)

  const loadFile = useCallback(async (type: string) => {
    setFileLoading(true)
    setFileError(null)
    setFileDirty(false)
    try {
      const r = await ReadClientConfigFile(type)
      setFileContent(r?.content || '')
      setFileMeta({ path: r?.path || '', format: r?.format || '', existed: !!r?.existed })
      setFileLoadKey(k => k + 1)
      setHasBackup(await HasClientConfigBackup(type))
    } catch (err: any) {
      setFileError(String(err?.message || err))
      setFileContent('')
      setFileMeta(null)
    } finally {
      setFileLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [])

  // 切 Tab 时同步加载该 client 的主配置文件原文
  useEffect(() => {
    if (activeType) loadFile(activeType)
  }, [activeType, loadFile])

  // 文件 + format 就绪后，从 Settings 拉一次当前 agent 的映射，silent 注入到预览
  // 为什么不在 ModelMappingSection 内做：子组件 GetSettings 与父组件 loadFile 是异步并行的，
  // 当 GetSettings 比 loadFile 先 resolve 时，子组件 patch 时 fileMeta 还为 null，直接 early return，
  // 之后 fileContent 被 loadFile 覆盖、注入丢失。父组件等 fileMeta 就绪后再 patch，时序确定。
  useEffect(() => {
    if (!fileMeta?.format || !activeType) return
    let cancelled = false
    GetSettings().then(s => {
      if (cancelled) return
      const all = ((s?.model_mappings || {}) as Record<string, Record<string, string>>)
      let bucket: Record<string, string> | undefined = all[activeType]
      if (!bucket && activeType === 'claude' && s?.model_mapping && Object.keys(s.model_mapping).length > 0) {
        bucket = s.model_mapping
      }
      const fmt = fileMeta.format
      setFileContent(prev => patchMappingPreview(prev, fmt, bucket || {}, activeType))
    }).catch(() => {})
    return () => { cancelled = true }
  }, [fileMeta?.format, activeType, fileLoadKey])

  const activeCfg = useMemo(
    () => configs.find(c => c.type === activeType),
    [configs, activeType]
  )

  // 一键配置：纯前端操作，只更新编辑器内容，不落盘。用户需手动点"保存"才写磁盘。
  // 注入 base_url/auth 后，优先用内存中未保存的 pendingMapping，否则从 Settings 读取已保存映射。
  const handleApply = async (cfg: ClientConfig) => {
    if (!fileMeta?.format) return
    const fmt = fileMeta.format
    let apiKey = QODER_API_KEY
    let bucket: Record<string, string> | undefined = pendingMapping?.[cfg.type]
    try {
      const s = await GetSettings()
      apiKey = s?.bridge_token || QODER_API_KEY
      if (!bucket) {
        const all = ((s?.model_mappings || {}) as Record<string, Record<string, string>>)
        bucket = all[cfg.type]
        if (!bucket && cfg.type === 'claude' && s?.model_mapping && Object.keys(s.model_mapping).length > 0) {
          bucket = s.model_mapping
        }
      }
    } catch { /* 拉取失败则使用默认值 */ }
    let applied = applyConfigToContent(fileContent, fmt, cfg.type, status.port, apiKey)
    applied = patchMappingPreview(applied, fmt, bucket || {}, cfg.type)
    setFileContent(applied)
    setFileDirty(true)
  }

  // 移除配置：同上，只更新编辑器内容，不落盘。
  const handleRemove = (cfg: ClientConfig) => {
    if (!fileMeta?.format) return
    const next = removeConfigFromContent(fileContent, fileMeta.format, cfg.type)
    setFileContent(next)
    setFileDirty(true)
  }

  // 还原：从备份恢复到上次保存前的状态，直接落盘后重新加载，不需要再点保存
  const handleRestore = async () => {
    if (!activeType) return
    try {
      await RestoreClientConfigFile(activeType)
      setFileDirty(false)
      setMappingDirty(false)
      await refresh()
      await loadFile(activeType)
    } catch (err: any) {
      setFileError(String(err?.message || err))
    }
  }

  // 重新加载：丢弃未保存的编辑，从磁盘重新读取
  const handleReload = () => {
    setFileDirty(false)
    setMappingDirty(false)
    loadFile(activeType)
  }

  // 统一保存：同时写 CLI 配置文件 + 持久化模型映射到 Settings.json
  const handleSaveFile = async () => {
    if (!activeType) return
    setFileSaving(true)
    setFileError(null)
    try {
      // 1. 写文件（仅当文件有改动时）— 落盘前剥离 qoder2api 旧式预览字段（claude 的三个槽位是合法字段，不剥离）
      if (fileDirty) {
        const cleaned = stripMappingPreview(fileContent, fileMeta?.format || '')
        await SaveClientConfigFile(activeType, cleaned)
        setFileDirty(false)
      }
      // 2. 写 Settings.ModelMappings（仅当映射有改动时）
      if (mappingDirty && pendingMapping) {
        const cur = await GetSettings()
        const merged = new account.Settings({
          ...(cur || {}),
          model_mapping: undefined,
          model_mappings: pendingMapping,
        } as any)
        await SaveSettings(merged)
        setMappingDirty(false)
      }
      await refresh()
      await loadFile(activeType) // loadFile 内部会更新 hasBackup
    } catch (err: any) {
      setFileError(String(err?.message || err))
    } finally {
      setFileSaving(false)
    }
  }

  const handleFormatFile = () => {
    if (!fileMeta?.format) return
    try {
      const formatted = formatContent(fileContent, fileMeta.format)
      if (formatted !== fileContent) {
        setFileContent(formatted)
        setFileDirty(true)
      }
      setFileError(null)
      setFormatStatus('done')
      setTimeout(() => setFormatStatus('idle'), 1500)
    } catch (err: any) {
      setFileError(`格式化失败: ${String(err?.message || err)}`)
      setFormatStatus('error')
      setTimeout(() => setFormatStatus('idle'), 1500)
    }
  }

  // 子组件 ModelMappingSection 在用户编辑/默认填充时调用：把当前 agent 的映射 patch 到预览（标 dirty）
  const handleMappingPatchPreview = useCallback((agentBucket: Record<string, string>) => {
    if (!fileMeta?.format) return
    const fmt = fileMeta.format
    setFileContent(prev => {
      const next = patchMappingPreview(prev, fmt, agentBucket, activeType)
      if (next !== prev) setFileDirty(true)
      return next
    })
  }, [fileMeta?.format, activeType])

  if (loading) return <div className="config-loading">加载中…</div>

  return (
    <div className="client-config-page">
      <div className="page-header">
        <h2>Agent 配置</h2>
      </div>

      {/* 横向 Tab 栏（紧贴页头之下，对齐 Sidebar 风格） */}
      <div className="client-tabs" role="tablist">
        {configs.map(cfg => {
          const iconSrc = ICON_BY_TYPE[cfg.type]
          return (
            <button
              key={cfg.type}
              role="tab"
              aria-selected={activeType === cfg.type}
              className={activeType === cfg.type ? 'active' : ''}
              onClick={() => setActiveType(cfg.type)}
              title={cfg.name}
            >
              {iconSrc ? <img src={iconSrc} alt="" className="tab-icon" /> : <span className="tab-icon" aria-hidden="true">{cfg.icon}</span>}
              <span>{cfg.name}</span>
              {cfg.applied && <span className="tab-applied-dot" aria-label="已配置" />}
            </button>
          )
        })}
      </div>

      {/* 当前 Tab 配置卡片 */}
      {activeCfg && (
        <div className={`config-card ${activeCfg.applied ? 'applied' : ''}`}>
          <div className="config-card-header">
            {ICON_BY_TYPE[activeCfg.type]
              ? <img src={ICON_BY_TYPE[activeCfg.type]} alt="" className="config-card-icon" />
              : <div className="config-card-icon-emoji">{activeCfg.icon}</div>}
            <div>
              <h3>{activeCfg.name}</h3>
              <p>{activeCfg.applied ? '已配置（由 qoder2api 管理）' : '未配置'}</p>
            </div>
            <div className={`config-status-dot ${activeCfg.applied ? 'active' : ''}`} />
          </div>

          <div className="config-card-body">
            {activeCfg.error && (
              <div className="config-warning">
                <AlertTriangle size={15} style={{ flexShrink: 0, marginTop: 1 }} />
                <span>{activeCfg.error}</span>
              </div>
            )}

            {/* 模型映射（按 agent 分组），改动暂存，"保存"按钮统一落盘 */}
            <ModelMappingSection
              agent={activeCfg.type}
              qoderModels={qoderModels}
              onMappingChange={(allMappings) => { setPendingMapping(allMappings); setMappingDirty(true) }}
              onMappingClean={() => setMappingDirty(false)}
              onPatchPreview={handleMappingPatchPreview}
            />

            {/* 配置文件编辑器（同卡片内分段） */}
            <div className="config-section-divider">
              <span className="section-title">📝 配置文件</span>

              <code className="section-path">{activeCfg.config_path}</code>
              <span className="meta" style={!fileDirty && fileMeta?.existed ? { color: 'var(--bs-success, #198754)' } : undefined}>
                {fileMeta && !fileMeta.existed && '· 文件不存在'}
                {fileDirty ? ' · 未保存' : (fileMeta?.existed ? ' · 已保存' : '')}
              </span>
              <span className="divider-spacer" />
              {fileMeta?.format === 'json' && (
                <button
                  key={formatStatus}
                  className={`icon-btn icon-btn-format${formatStatus === 'error' ? ' icon-btn-danger' : ''}${formatStatus === 'done' ? ' btn-format-flash' : ''}`}
                  onClick={handleFormatFile}
                  disabled={fileLoading || !fileContent.trim() || formatStatus !== 'idle'}
                  title="格式化（仅 JSON）"
                >
                  <WandIcon
                    key={formatStatus}
                    size={18}
                    className={formatStatus === 'done' ? 'wand-icon-animate' : ''}
                  />
                </button>
              )}
              <button
                className="icon-btn icon-btn-apply"
                onClick={() => handleApply(activeCfg)}
                disabled={applying === activeCfg.type}
                title={activeCfg.applied ? '更新配置' : '一键配置'}
              ><Rocket size={18} /></button>
              <button
                className="icon-btn icon-btn-reload"
                onClick={handleReload}
                disabled={!fileDirty && !mappingDirty}
                title="丢弃未保存的编辑，从磁盘重新加载"
              ><FolderSync size={18} /></button>
              <button
                className="icon-btn btn-primary"
                onClick={handleSaveFile}
                disabled={(!fileDirty && !mappingDirty) || fileSaving}
                title="保存"
              ><Save size={18} /></button>
              <button
                className="icon-btn icon-btn-restore"
                onClick={handleRestore}
                disabled={!(activeCfg.applied && hasBackup)}
                title="还原到首次保存前的配置（恢复备份）"
              ><History size={18} /></button>
            </div>
            {fileError && (
              <div className="config-warning" style={{ marginTop: 0, marginBottom: 6 }}>
                <AlertTriangle size={15} style={{ flexShrink: 0, marginTop: 1 }} />
                <span>{fileError}</span>
              </div>
            )}
            <div style={{ marginTop: 10 }}>
            <ConfigEditor
              ref={editorRef}
              value={fileContent}
              onChange={v => { setFileContent(v); setFileDirty(true) }}
              format={fileMeta?.format as 'json' | 'toml' | 'dotenv' | undefined}
              placeholderText={fileLoading ? '加载中…' : '配置文件内容…'}
              minLines={16}
            />
            </div>

          </div>
        </div>
      )}

    </div>
  )
}

// ============== ModelMappingSection ==============
// 仅本 agent 的模型映射卡片（嵌入 ClientConfigPage 的 config-card-body 中）
//
// 数据形态：Settings.ModelMappings = { claude: {sonnet: performance, ...}, codex: {...}, gemini: {...} }
// 兼容老数据：Settings.ModelMapping (扁平 map) 仅当 ModelMappings[claude] 不存在时迁移到 claude 桶。
//
// 行为：
//   - 改动行 → 调用 onMappingChange(allMappings) 通知父组件 dirty + 完整映射快照
//   - 不直接调用 SaveSettings；落盘由父组件的"保存"按钮统一处理（同时保存 CLI 配置文件 + Settings）
//   - 模型映射是 bridge 内部转换层，不写入 CLI 配置文件本体

type Row = { id: number; from: string; to: string }
let _rid = 1
const newRid = () => _rid++

interface MappingProps {
  agent: string
  qoderModels: main.QoderModel[]
  onMappingChange: (allMappings: Record<string, Record<string, string>>) => void
  onMappingClean: () => void
  onPatchPreview: (agentBucket: Record<string, string>) => void
}

function ModelMappingSection({ agent, qoderModels, onMappingChange, onMappingClean, onPatchPreview }: MappingProps) {
  const [allMappings, setAllMappings] = useState<Record<string, Record<string, string>>>({})
  const [rows, setRows] = useState<Row[]>([])
  const [error, setError] = useState<string | null>(null)
  const [touched, setTouched] = useState(false)

  // 加载 / agent 切换时重新构造 rows（仅维护本组件状态；预览注入由父组件统一处理，避免竞态）
  useEffect(() => {
    let cancelled = false
    GetSettings().then(s => {
      if (cancelled) return
      const mappings = ((s?.model_mappings || {}) as Record<string, Record<string, string>>)
      let bucket: Record<string, string> | undefined = mappings[agent]
      if (!bucket && agent === 'claude' && s?.model_mapping && Object.keys(s.model_mapping).length > 0) {
        bucket = s.model_mapping
      }
      const list = bucket ? Object.entries(bucket).map(([from, to]) => ({ id: newRid(), from, to })) : []
      setAllMappings(mappings)
      setRows(list)
      setError(null)
      setTouched(false)
      // 初始化时也通知父组件当前映射快照，确保 pendingMapping 始终反映最新状态
      const merged = { ...mappings }
      if (bucket && Object.keys(bucket).length > 0) merged[agent] = bucket
      else delete merged[agent]
      onMappingChange(merged)
      onMappingClean()
    }).catch(err => setError(String(err?.message || err)))
    return () => { cancelled = true }
  }, [agent])

  // rows 变化 → 重组 allMappings 并通知父
  useEffect(() => {
    if (!touched) return
    const next: Record<string, string> = {}
    for (const r of rows) {
      const k = r.from.trim(), v = r.to.trim()
      if (k && v) next[k] = v
    }
    const merged = { ...allMappings }
    if (Object.keys(next).length === 0) delete merged[agent]
    else merged[agent] = next
    onMappingChange(merged)
    // 同步 patch 到配置文件预览（仅展示，保存时会被 strip）
    onPatchPreview(next)
  }, [rows, touched])

  const candidates = CLIENT_MODEL_OPTIONS_BY_AGENT[agent] || []

  const markTouched = () => { if (!touched) setTouched(true) }
  const addRow = () => { setRows([...rows, { id: newRid(), from: '', to: '' }]); markTouched() }
  const updateRow = (id: number, key: 'from' | 'to', v: string) => {
    setRows(rows.map(r => r.id === id ? { ...r, [key]: v } : r)); markTouched()
  }
  const removeRow = (id: number) => { setRows(rows.filter(r => r.id !== id)); markTouched() }
  const fillDefaults = () => {
    const existing = new Set(rows.map(r => r.from.trim()))
    const adds = (DEFAULT_MAPPING_BY_AGENT[agent] || [])
      .filter(([from]) => !existing.has(from))
      .map(([from, to]) => ({ id: newRid(), from, to }))
    if (adds.length > 0) { setRows([...rows, ...adds]); markTouched() }
  }

  return (
    <div className="mapping-section">
      <div className="mapping-section-header">
        <span className="section-title">🔀 模型映射</span>
        <span className="meta">客户端模型名 → Qoder model.key</span>
        <span className="divider-spacer" />
        <button className="btn btn-secondary btn-sm" onClick={fillDefaults} title="按家族关键字一键填充默认条目">默认</button>
        <button className="btn btn-secondary btn-sm" onClick={addRow}>+ 加一行</button>
      </div>

      {error && (
        <div className="config-warning">
          <AlertTriangle size={15} style={{ flexShrink: 0, marginTop: 1 }} />
          <span>{error}</span>
        </div>
      )}

      {rows.length === 0 ? (
        <div className="mapping-empty">未配置自定义映射，将使用内置默认表（{(DEFAULT_MAPPING_BY_AGENT[agent] || []).map(([f, t]) => `${f}→${t}`).join(' / ') || '无'}）</div>
      ) : (
        <div className="mapping-list">
          {rows.map(r => (
            <div key={r.id} className="mapping-row">
              <MappingSelect
                value={r.from}
                onChange={v => updateRow(r.id, 'from', v)}
                options={candidates.map(c => ({ value: c, label: c }))}
                placeholder="客户端模型名"
              />
              <svg className="mapping-arrow-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" width="14" height="14">
                <line x1="5" y1="12" x2="19" y2="12"/>
                <polyline points="12 5 19 12 12 19"/>
              </svg>
              {qoderModels.length > 0 ? (
                <MappingSelect
                  value={r.to}
                  onChange={v => updateRow(r.id, 'to', v)}
                  options={[
                    ...qoderModels.map(m => ({ value: m.key, label: `${m.display_name} (${m.key})${m.is_default ? ' · 默认' : ''}` })),
                    ...(r.to && !qoderModels.some(m => m.key === r.to) ? [{ value: r.to, label: `${r.to}（自定义/已下线）` }] : [])
                  ]}
                />
              ) : (
                <MappingSelect
                  value={r.to}
                  onChange={v => updateRow(r.id, 'to', v)}
                  options={FALLBACK_QODER_KEYS.map(k => ({ value: k, label: k }))}
                />
              )}
              <button className="mapping-delete-btn" onClick={() => removeRow(r.id)} title="删除此行" aria-label="删除">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" width="13" height="13">
                  <line x1="18" y1="6" x2="6" y2="18"/>
                  <line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
