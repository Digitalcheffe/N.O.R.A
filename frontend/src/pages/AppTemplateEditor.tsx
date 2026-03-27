import { useEffect, useRef, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { EditorView, lineNumbers, highlightActiveLine } from '@codemirror/view'
import { EditorState } from '@codemirror/state'
import { yaml } from '@codemirror/lang-yaml'
import { appTemplates } from '../api/client'
import type { ValidationResult } from '../api/types'

// ── NORA dark theme for CodeMirror ─────────────────────────────────────────

const noraTheme = EditorView.theme(
  {
    '&': {
      backgroundColor: 'var(--bg2)',
      color: 'var(--text)',
      fontSize: '13px',
      fontFamily: 'var(--mono)',
      height: '100%',
      minHeight: '400px',
    },
    '.cm-content': {
      caretColor: 'var(--accent)',
      padding: '12px 0',
    },
    '.cm-gutters': {
      backgroundColor: 'var(--bg3)',
      color: 'var(--text3)',
      border: 'none',
      borderRight: '1px solid var(--border)',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'var(--bg4)',
    },
    '.cm-activeLine': {
      backgroundColor: 'rgba(59,130,246,0.05)',
    },
    '.cm-cursor': {
      borderLeftColor: 'var(--accent)',
    },
    '.cm-selectionBackground': {
      backgroundColor: 'var(--accent-dim) !important',
    },
    '&.cm-focused .cm-selectionBackground': {
      backgroundColor: 'var(--accent-dim) !important',
    },
    '.cm-line': {
      paddingLeft: '8px',
    },
  },
  { dark: true },
)

// ── Default YAML template shown in a new editor ─────────────────────────────

const DEFAULT_YAML = `meta:
  name: My Custom App
  category: Custom
  logo: ""
  description: A brief description of this app
  capability: webhook_only

webhook:
  setup_instructions: |
    Configure your app to POST events to {base_url}/ingest/{token}
  recommended_events: []
  field_mappings:
    event_type: "$.eventType"
    message: "$.message"
  display_template: "{event_type} — {message}"
  severity_field: event_type
  severity_mapping:
    error: error
    info: info
    warn: warn

monitor:
  check_type: url
  check_url: "{base_url}/health"
  healthy_status: 200
  check_interval: 5m

digest:
  categories: []
`

// ── Minimal YAML parser for preview (key: value, indented blocks) ───────────

interface ParsedAppTemplate {
  name: string
  category: string
  description: string
  capability: string
  logo: string
  displayTemplate: string
  fieldMappings: Record<string, string>
  severityMapping: Record<string, string>
}

function parseAppTemplateYAML(content: string): ParsedAppTemplate | null {
  try {
    const lines = content.split('\n')
    const result: ParsedAppTemplate = {
      name: '',
      category: '',
      description: '',
      capability: '',
      logo: '',
      displayTemplate: '',
      fieldMappings: {},
      severityMapping: {},
    }

    let section = ''
    let subSection = ''

    for (const line of lines) {
      const trimmed = line.trimEnd()
      if (!trimmed || trimmed.startsWith('#')) continue

      // Top-level section headers
      if (/^meta:/.test(trimmed)) { section = 'meta'; subSection = ''; continue }
      if (/^webhook:/.test(trimmed)) { section = 'webhook'; subSection = ''; continue }
      if (/^monitor:/.test(trimmed)) { section = 'monitor'; subSection = ''; continue }
      if (/^digest:/.test(trimmed)) { section = 'digest'; subSection = ''; continue }

      // Sub-section headers (2-space indent)
      if (/^  field_mappings:/.test(trimmed)) { subSection = 'field_mappings'; continue }
      if (/^  severity_mapping:/.test(trimmed)) { subSection = 'severity_mapping'; continue }
      if (/^  (recommended_events|not_recommended|categories):/.test(trimmed)) { subSection = 'list'; continue }

      const kv = trimmed.match(/^(\s*)(\S[^:]*?):\s*(.*)$/)
      if (!kv) continue
      const indent = kv[1].length
      const key = kv[2].trim()
      const val = kv[3].replace(/^["']|["']$/g, '').trim()

      if (section === 'meta' && indent === 2) {
        if (key === 'name') result.name = val
        else if (key === 'category') result.category = val
        else if (key === 'description') result.description = val
        else if (key === 'capability') result.capability = val
        else if (key === 'logo') result.logo = val
      } else if (section === 'webhook') {
        if (indent === 2 && key === 'display_template') result.displayTemplate = val
        else if (subSection === 'field_mappings' && indent === 4) result.fieldMappings[key] = val
        else if (subSection === 'severity_mapping' && indent === 4) result.severityMapping[key] = val
      }
    }

    return result
  } catch {
    return null
  }
}

// ── Placeholder substitution for display template preview ──────────────────

const PLACEHOLDER_VALUES: Record<string, string> = {
  event_type: 'Download',
  series_title: 'Breaking Bad',
  episode_title: 'Ozymandias',
  season_number: '5',
  episode_number: '14',
  quality: '1080p',
  message: 'Item downloaded successfully',
  title: 'Breaking Bad',
  artist: 'Metallica',
  album: 'Metallica',
  version: '1.2.3',
  hostname: 'nas01',
  status: 'success',
}

function renderTemplate(template: string, fieldMappings: Record<string, string>): string {
  if (!template) return '—'
  let result = template
  for (const field of Object.keys(fieldMappings)) {
    const val = PLACEHOLDER_VALUES[field] ?? field
    result = result.replaceAll(`{${field}}`, val)
  }
  // Replace any remaining {tokens} with their name
  result = result.replace(/\{([^}]+)\}/g, (_, tok) => PLACEHOLDER_VALUES[tok] ?? tok)
  return result
}

// ── Capability badge colours ────────────────────────────────────────────────

const CAPABILITY_COLORS: Record<string, string> = {
  full: 'var(--green)',
  webhook_only: 'var(--accent)',
  monitor_only: 'var(--yellow)',
  docker_only: 'var(--text2)',
  limited: 'var(--text3)',
}

// ── AppTemplateEditor component ─────────────────────────────────────────────

export function AppTemplateEditor() {
  const navigate = useNavigate()
  const editorRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)

  const [validation, setValidation] = useState<ValidationResult | null>(null)
  const [preview, setPreview] = useState<ParsedAppTemplate | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  // Debounce timer ref
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const handleDocChange = useCallback((content: string) => {
    // Update preview immediately (cheap client-side parse)
    setPreview(parseAppTemplateYAML(content))

    // Debounce validation API call
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(async () => {
      try {
        const result = await appTemplates.validate(content)
        setValidation(result)
      } catch {
        // Network error — don't show stale validation
        setValidation(null)
      }
    }, 500)
  }, [])

  // Initialise CodeMirror
  useEffect(() => {
    if (!editorRef.current) return

    const startState = EditorState.create({
      doc: DEFAULT_YAML,
      extensions: [
        lineNumbers(),
        highlightActiveLine(),
        yaml(),
        noraTheme,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            handleDocChange(update.state.doc.toString())
          }
        }),
        EditorView.lineWrapping,
      ],
    })

    const view = new EditorView({
      state: startState,
      parent: editorRef.current,
    })
    viewRef.current = view

    // Run initial parse + validation
    handleDocChange(DEFAULT_YAML)

    return () => {
      view.destroy()
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleSave = async () => {
    if (!viewRef.current) return
    const content = viewRef.current.state.doc.toString()

    setSaving(true)
    setSaveError(null)
    try {
      await appTemplates.createCustom(content)
      navigate('/settings?tab=apps')
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  const capColor = preview?.capability
    ? (CAPABILITY_COLORS[preview.capability] ?? 'var(--text2)')
    : 'var(--text2)'

  return (
    <div style={styles.page}>
      {/* Header */}
      <div style={styles.header}>
        <div>
          <h1 style={styles.title}>Custom App Template Editor</h1>
          <p style={styles.subtitle}>Write a YAML app template for an app not in the library</p>
        </div>
        <div style={styles.headerActions}>
          {validation && (
            <span
              style={{
                ...styles.badge,
                background: validation.valid ? 'var(--green-dim)' : 'var(--red-dim)',
                color: validation.valid ? 'var(--green)' : 'var(--red)',
              }}
            >
              {validation.valid ? 'Valid' : 'Invalid'}
            </span>
          )}
          <button
            style={styles.cancelBtn}
            onClick={() => navigate('/settings?tab=apps')}
          >
            Cancel
          </button>
          <button
            style={{
              ...styles.saveBtn,
              opacity: saving ? 0.6 : 1,
              cursor: saving ? 'not-allowed' : 'pointer',
            }}
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? 'Saving…' : 'Save App Template'}
          </button>
        </div>
      </div>

      {/* Two-panel layout */}
      <div style={styles.panels}>
        {/* Left: YAML editor */}
        <div style={styles.leftPanel}>
          <div style={styles.panelLabel}>YAML Editor</div>
          <div style={styles.editorWrap} ref={editorRef} />
          {/* Validation errors */}
          {validation && !validation.valid && validation.errors.length > 0 && (
            <div style={styles.errorList}>
              {validation.errors.map((e, i) => (
                <div key={i} style={styles.errorLine}>
                  {e}
                </div>
              ))}
            </div>
          )}
          {saveError && (
            <div style={styles.errorList}>
              <div style={styles.errorLine}>{saveError}</div>
            </div>
          )}
        </div>

        {/* Right: live preview */}
        <div style={styles.rightPanel}>
          <div style={styles.panelLabel}>Live Preview</div>

          {/* App card */}
          <div style={styles.card}>
            <div style={styles.cardHeader}>
              <div style={styles.appIcon}>
                {preview?.logo ? (
                  <img src={preview.logo} alt="" style={styles.logoImg} />
                ) : (
                  <span style={styles.iconPlaceholder}>
                    {(preview?.name ?? '?')[0]?.toUpperCase() ?? '?'}
                  </span>
                )}
              </div>
              <div style={styles.cardMeta}>
                <div style={styles.appName}>{preview?.name || 'Untitled'}</div>
                {preview?.capability && (
                  <span style={{ ...styles.capBadge, color: capColor, borderColor: capColor }}>
                    {preview.capability}
                  </span>
                )}
              </div>
            </div>
            <div style={styles.appDesc}>
              {preview?.description || 'No description yet'}
            </div>
          </div>

          {/* Display template preview */}
          {preview?.displayTemplate && (
            <div style={styles.section}>
              <div style={styles.sectionLabel}>Display Template Preview</div>
              <div style={styles.templatePreview}>
                {renderTemplate(preview.displayTemplate, preview.fieldMappings)}
              </div>
              <div style={styles.templateRaw}>{preview.displayTemplate}</div>
            </div>
          )}

          {/* Field mappings */}
          {Object.keys(preview?.fieldMappings ?? {}).length > 0 && (
            <div style={styles.section}>
              <div style={styles.sectionLabel}>Field Mappings</div>
              <div style={styles.mappingTable}>
                {Object.entries(preview!.fieldMappings).map(([tag, path]) => (
                  <div key={tag} style={styles.mappingRow}>
                    <span style={styles.mappingTag}>{tag}</span>
                    <span style={styles.mappingArrow}>→</span>
                    <span style={styles.mappingPath}>{path}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Severity mapping */}
          {Object.keys(preview?.severityMapping ?? {}).length > 0 && (
            <div style={styles.section}>
              <div style={styles.sectionLabel}>Severity Mapping</div>
              <table style={styles.severityTable}>
                <thead>
                  <tr>
                    <th style={styles.th}>Value</th>
                    <th style={styles.th}>Severity</th>
                  </tr>
                </thead>
                <tbody>
                  {Object.entries(preview!.severityMapping).map(([val, sev]) => (
                    <tr key={val}>
                      <td style={styles.td}>{val}</td>
                      <td style={{ ...styles.td, color: severityColor(sev) }}>{sev}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function severityColor(sev: string): string {
  switch (sev) {
    case 'error':
    case 'critical':
      return 'var(--red)'
    case 'warn':
      return 'var(--yellow)'
    case 'info':
      return 'var(--accent)'
    default:
      return 'var(--text2)'
  }
}

// ── Styles ──────────────────────────────────────────────────────────────────

const styles: Record<string, React.CSSProperties> = {
  page: {
    padding: '24px',
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
    height: '100%',
    boxSizing: 'border-box',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: '16px',
  },
  title: {
    margin: 0,
    fontSize: '20px',
    fontWeight: 600,
    color: 'var(--text)',
    fontFamily: 'var(--sans)',
  },
  subtitle: {
    margin: '4px 0 0',
    fontSize: '13px',
    color: 'var(--text2)',
    fontFamily: 'var(--sans)',
  },
  headerActions: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  badge: {
    padding: '4px 10px',
    borderRadius: '4px',
    fontSize: '12px',
    fontWeight: 600,
    fontFamily: 'var(--mono)',
  },
  cancelBtn: {
    padding: '8px 18px',
    background: 'transparent',
    color: 'var(--red)',
    border: '1px solid var(--red)',
    borderRadius: '6px',
    fontSize: '13px',
    fontWeight: 500,
    fontFamily: 'var(--sans)',
    cursor: 'pointer',
  },
  saveBtn: {
    padding: '8px 18px',
    background: 'var(--accent)',
    color: '#fff',
    border: 'none',
    borderRadius: '6px',
    fontSize: '13px',
    fontWeight: 600,
    fontFamily: 'var(--sans)',
  },
  panels: {
    display: 'flex',
    gap: '16px',
    flex: 1,
    minHeight: 0,
  },
  leftPanel: {
    flex: '0 0 60%',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    minHeight: 0,
  },
  rightPanel: {
    flex: '0 0 40%',
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    overflowY: 'auto',
  },
  panelLabel: {
    fontSize: '11px',
    fontWeight: 600,
    letterSpacing: '0.08em',
    textTransform: 'uppercase',
    color: 'var(--text3)',
    fontFamily: 'var(--sans)',
  },
  editorWrap: {
    flex: 1,
    border: '1px solid var(--border)',
    borderRadius: '6px',
    overflow: 'hidden',
    minHeight: 0,
  },
  errorList: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  errorLine: {
    fontFamily: 'var(--mono)',
    fontSize: '12px',
    color: 'var(--red)',
    background: 'var(--red-dim)',
    padding: '4px 8px',
    borderRadius: '4px',
  },
  card: {
    background: 'var(--bg3)',
    border: '1px solid var(--border)',
    borderRadius: '8px',
    padding: '14px',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  cardHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  appIcon: {
    width: '40px',
    height: '40px',
    borderRadius: '8px',
    background: 'var(--bg4)',
    border: '1px solid var(--border)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    flexShrink: 0,
    overflow: 'hidden',
  },
  logoImg: {
    width: '100%',
    height: '100%',
    objectFit: 'cover',
  },
  iconPlaceholder: {
    fontSize: '18px',
    fontWeight: 700,
    color: 'var(--text2)',
    fontFamily: 'var(--mono)',
  },
  cardMeta: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  appName: {
    fontSize: '15px',
    fontWeight: 600,
    color: 'var(--text)',
    fontFamily: 'var(--sans)',
  },
  capBadge: {
    fontSize: '11px',
    fontWeight: 500,
    fontFamily: 'var(--mono)',
    border: '1px solid',
    borderRadius: '4px',
    padding: '1px 6px',
    display: 'inline-block',
  },
  appDesc: {
    fontSize: '13px',
    color: 'var(--text2)',
    fontFamily: 'var(--sans)',
    lineHeight: 1.5,
  },
  section: {
    background: 'var(--bg3)',
    border: '1px solid var(--border)',
    borderRadius: '8px',
    padding: '12px 14px',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  sectionLabel: {
    fontSize: '11px',
    fontWeight: 600,
    letterSpacing: '0.08em',
    textTransform: 'uppercase',
    color: 'var(--text3)',
    fontFamily: 'var(--sans)',
  },
  templatePreview: {
    fontFamily: 'var(--mono)',
    fontSize: '13px',
    color: 'var(--text)',
    background: 'var(--bg4)',
    padding: '8px 10px',
    borderRadius: '4px',
  },
  templateRaw: {
    fontFamily: 'var(--mono)',
    fontSize: '11px',
    color: 'var(--text3)',
  },
  mappingTable: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  mappingRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontFamily: 'var(--mono)',
    fontSize: '12px',
  },
  mappingTag: {
    color: 'var(--accent)',
    minWidth: '100px',
  },
  mappingArrow: {
    color: 'var(--text3)',
  },
  mappingPath: {
    color: 'var(--text2)',
  },
  severityTable: {
    width: '100%',
    borderCollapse: 'collapse',
    fontFamily: 'var(--mono)',
    fontSize: '12px',
  },
  th: {
    textAlign: 'left' as const,
    color: 'var(--text3)',
    fontWeight: 500,
    padding: '4px 8px',
    borderBottom: '1px solid var(--border)',
  },
  td: {
    color: 'var(--text)',
    padding: '4px 8px',
  },
}
