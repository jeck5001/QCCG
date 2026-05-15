import React, { useRef, useEffect, useMemo, useImperativeHandle, forwardRef } from 'react'
import { EditorView, basicSetup } from 'codemirror'
import { json } from '@codemirror/lang-json'
import { EditorState } from '@codemirror/state'
import { placeholder } from '@codemirror/view'
import { linter, Diagnostic } from '@codemirror/lint'

export interface ConfigEditorHandle {
  /** 格式化当前内容（仅 JSON），返回是否成功 */
  formatContent: () => boolean
}

interface ConfigEditorProps {
  value: string
  onChange: (value: string) => void
  format?: 'json' | 'toml' | 'dotenv'
  placeholderText?: string
  minLines?: number
}

const baseTheme = EditorView.baseTheme({
  '&': {
    border: '1px solid var(--bs-border-color, #dee2e6)',
    borderRadius: '6px',
    fontSize: '13px',
    background: 'var(--bs-body-bg, #fff)',
  },
  '&.cm-focused': {
    outline: 'none',
    borderColor: '#86b7fe',
    boxShadow: '0 0 0 0.25rem rgba(13,110,253,.25)',
  },
  '.cm-scroller': {
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Courier New', monospace",
    overflow: 'auto',
  },
  '.cm-gutters': {
    background: 'var(--bs-tertiary-bg, #f8f9fa)',
    borderRight: '1px solid var(--bs-border-color, #dee2e6)',
    color: '#6c757d',
  },
  '.cm-activeLine': { background: 'rgba(13,110,253,.04)' },
  '.cm-activeLineGutter': { background: 'rgba(13,110,253,.04)' },
  '.cm-selectionBackground, .cm-content ::selection': {
    background: 'rgba(13,110,253,.15) !important',
  },
})

const ConfigEditor = forwardRef<ConfigEditorHandle, ConfigEditorProps>(({
  value,
  onChange,
  format = 'json',
  placeholderText = '',
  minLines = 8,
}, ref) => {
  const editorRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  useEffect(() => { onChangeRef.current = onChange }, [onChange])

  useImperativeHandle(ref, () => ({
    formatContent() {
      const view = viewRef.current
      if (!view || format !== 'json') return false
      const raw = view.state.doc.toString()
      if (!raw.trim()) return false
      try {
        const formatted = JSON.stringify(JSON.parse(raw), null, 2) + '\n'
        view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: formatted } })
        onChangeRef.current(formatted)
        return true
      } catch {
        return false
      }
    },
  }), [format])

  const jsonLinter = useMemo(() => linter((view) => {
    if (format !== 'json') return []
    const doc = view.state.doc.toString()
    if (!doc.trim()) return []
    const diagnostics: Diagnostic[] = []
    try { JSON.parse(doc) } catch (e) {
      diagnostics.push({
        from: 0, to: doc.length, severity: 'error',
        message: e instanceof SyntaxError ? e.message : 'Invalid JSON',
      })
    }
    return diagnostics
  }), [format])

  const suppressRef = useRef(false)

  useEffect(() => {
    if (!editorRef.current) return
    const minHeightPx = minLines * 20
    const extensions = [
      basicSetup,
      ...(format === 'json' ? [json()] : []),
      placeholder(placeholderText),
      baseTheme,
      EditorView.theme({ '&': { minHeight: `${minHeightPx}px` }, '.cm-scroller': { overflow: 'auto' } }),
      jsonLinter,
      EditorView.updateListener.of((update) => {
        if (update.docChanged && !suppressRef.current) onChangeRef.current(update.state.doc.toString())
      }),
    ]
    const state = EditorState.create({ doc: value, extensions })
    const view = new EditorView({ state, parent: editorRef.current })
    viewRef.current = view
    return () => { view.destroy(); viewRef.current = null }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [format, minLines, jsonLinter])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    if (view.state.doc.toString() === value) return
    suppressRef.current = true
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
    suppressRef.current = false
  }, [value])

  return <div ref={editorRef} style={{ width: '100%' }} />
})

ConfigEditor.displayName = 'ConfigEditor'
export default ConfigEditor
