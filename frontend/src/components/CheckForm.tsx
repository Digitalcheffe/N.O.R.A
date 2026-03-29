import type { ReactNode } from 'react'
import type { MonitorCheck, CreateCheckInput, CheckType, SSLSource, InfraIntegration, TraefikCert } from '../api/types'
import './CheckForm.css'

// ── Types ─────────────────────────────────────────────────────────────────────

export type FormFields = {
  type: CheckType
  name: string
  target: string
  interval_secs: string
  expected_status: string
  ssl_warn_days: string
  ssl_crit_days: string
  ssl_source: SSLSource
  integration_id: string
  traefik_domain: string
  skip_tls_verify: string  // 'true' | 'false'
}

export const defaultForm: FormFields = {
  type: 'ping',
  name: '',
  target: '',
  interval_secs: '60',
  expected_status: '200',
  ssl_warn_days: '30',
  ssl_crit_days: '7',
  ssl_source: 'standalone',
  integration_id: '',
  traefik_domain: '',
  skip_tls_verify: 'false',
}

// ── Helpers ───────────────────────────────────────────────────────────────────

export function validateForm(form: FormFields): string | null {
  if (!form.name.trim()) return 'Name is required'
  const interval = parseInt(form.interval_secs, 10)
  if (isNaN(interval) || interval < 30) return 'Interval must be at least 30 seconds'

  if (form.type === 'url') {
    if (!form.target.trim()) return 'Target is required'
    if (!form.target.startsWith('http://') && !form.target.startsWith('https://')) {
      return 'Target must begin with http:// or https://'
    }
  }

  if (form.type === 'ssl') {
    if (form.ssl_source === 'traefik') {
      if (!form.traefik_domain) return 'Select a domain from the Traefik cert list'
    } else {
      if (!form.target.trim()) return 'Target is required'
      if (!form.target.startsWith('http://') && !form.target.startsWith('https://')) {
        return 'Target must begin with http:// or https://'
      }
    }
  }

  if (form.type === 'ping' && !form.target.trim()) return 'Target is required'

  return null
}

export function checkToForm(check: MonitorCheck): FormFields {
  return {
    type: check.type as CheckType,
    name: check.name,
    target: check.type === 'ssl' && check.ssl_source === 'traefik' ? '' : check.target,
    interval_secs: String(check.interval_secs),
    expected_status: String(check.expected_status ?? 200),
    ssl_warn_days: String(check.ssl_warn_days ?? 30),
    ssl_crit_days: String(check.ssl_crit_days ?? 7),
    ssl_source: (check.ssl_source as SSLSource) ?? 'standalone',
    integration_id: check.integration_id ?? '',
    traefik_domain: check.ssl_source === 'traefik' ? check.target : '',
    skip_tls_verify: check.skip_tls_verify ? 'true' : 'false',
  }
}

export function formToInput(form: FormFields, integrationID?: string): CreateCheckInput {
  const input: CreateCheckInput = {
    name: form.name.trim(),
    type: form.type,
    target: form.type === 'ssl' && form.ssl_source === 'traefik'
      ? form.traefik_domain
      : form.target.trim(),
    interval_secs: parseInt(form.interval_secs, 10),
  }
  if (form.type === 'url') {
    input.expected_status = parseInt(form.expected_status, 10)
    input.skip_tls_verify = form.skip_tls_verify === 'true'
  }
  if (form.type === 'ssl') {
    input.ssl_warn_days = parseInt(form.ssl_warn_days, 10)
    input.ssl_crit_days = parseInt(form.ssl_crit_days, 10)
    input.ssl_source = form.ssl_source
    if (form.ssl_source === 'traefik' && integrationID) {
      input.integration_id = integrationID
    }
  }
  return input
}

export function renderCheckResult(check: MonitorCheck): ReactNode {
  if (!check.last_result) return <span className="check-result-empty">No result recorded yet</span>
  try {
    const r = JSON.parse(check.last_result) as Record<string, unknown>
    if (check.type === 'ping') {
      return (
        <div className="check-result-grid">
          <span className="check-result-label">Latency</span>
          <span className="check-result-value">{r.latency_ms != null ? `${r.latency_ms}ms` : '—'}</span>
        </div>
      )
    }
    if (check.type === 'url') {
      return (
        <div className="check-result-grid">
          <span className="check-result-label">HTTP Status</span>
          <span className="check-result-value">{r.status_code != null ? String(r.status_code) : '—'}</span>
          <span className="check-result-label">Latency</span>
          <span className="check-result-value">{r.latency_ms != null ? `${r.latency_ms}ms` : '—'}</span>
          {!!r.error && <>
            <span className="check-result-label">Error</span>
            <span className="check-result-value check-result-error">{String(r.error)}</span>
          </>}
        </div>
      )
    }
    if (check.type === 'ssl') {
      if (r.error) return (
        <div className="check-result-grid">
          <span className="check-result-label">Error</span>
          <span className="check-result-value check-result-error">{String(r.error)}</span>
        </div>
      )
      const expiresStr = r.expires_at
        ? new Date(r.expires_at as string).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
        : '—'
      return (
        <div className="check-result-grid">
          <span className="check-result-label">Days remaining</span>
          <span className="check-result-value">{r.days_remaining != null ? String(r.days_remaining) : '—'}</span>
          <span className="check-result-label">Expires</span>
          <span className="check-result-value">{expiresStr}</span>
          {!!r.issuer && <>
            <span className="check-result-label">Issuer</span>
            <span className="check-result-value">{String(r.issuer)}</span>
          </>}
          {!!r.subject && <>
            <span className="check-result-label">Subject</span>
            <span className="check-result-value">{String(r.subject)}</span>
          </>}
        </div>
      )
    }
  } catch { /* fall through */ }
  return <span className="check-result-empty">{check.last_result}</span>
}

// ── CheckForm component ───────────────────────────────────────────────────────

interface CheckFormProps {
  form: FormFields
  onChange: (field: keyof FormFields, value: string) => void
  onSubmit: () => void
  onCancel: () => void
  error: string | null
  submitting: boolean
  title: string
  submitLabel: string
  extraAction?: ReactNode
  traefikIntegrations: InfraIntegration[]
  traefikCerts: TraefikCert[]
  onIntegrationChange: (integrationId: string) => void
}

const CHECK_TYPES: CheckType[] = ['ping', 'url', 'ssl']

export function CheckForm({
  form,
  onChange,
  onSubmit,
  onCancel,
  error,
  submitting,
  title,
  submitLabel,
  extraAction,
  traefikIntegrations,
  traefikCerts,
  onIntegrationChange,
}: CheckFormProps) {
  const hasTraefik = traefikIntegrations.length > 0
  const selectedIntegration = traefikIntegrations.find(i => i.id === form.integration_id)

  return (
    <div className="add-form">
      {title && <div className="form-title">{title}</div>}
      <div className="type-selector">
        {CHECK_TYPES.map(t => (
          <button
            key={t}
            className={`type-btn${form.type === t ? ' active' : ''}`}
            onClick={() => onChange('type', t)}
          >
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>
      <div className="form-fields">
        <div className="form-field">
          <div className="form-label">Name</div>
          <input
            className="form-input"
            value={form.name}
            onChange={e => onChange('name', e.target.value)}
            placeholder="e.g. Proxmox Web UI"
          />
        </div>

        {form.type === 'ssl' && (
          <div className="form-field">
            <div className="form-label">SSL Source</div>
            {!hasTraefik ? (
              <div className="ssl-no-traefik-banner">
                Connect Traefik in Settings → Integrations to enable automatic SSL discovery.
              </div>
            ) : (
              <div className="type-selector">
                <button
                  className={`type-btn${form.ssl_source === 'traefik' ? ' active' : ''}`}
                  onClick={() => onChange('ssl_source', 'traefik')}
                >
                  Traefik
                </button>
                <button
                  className={`type-btn${form.ssl_source === 'standalone' ? ' active' : ''}`}
                  onClick={() => onChange('ssl_source', 'standalone')}
                >
                  Standalone
                </button>
              </div>
            )}
          </div>
        )}

        {form.type === 'ssl' && form.ssl_source === 'traefik' && hasTraefik && (
          <>
            {traefikIntegrations.length > 1 && (
              <div className="form-field">
                <div className="form-label">Traefik Integration</div>
                <select
                  className="form-input"
                  value={form.integration_id}
                  onChange={e => {
                    onChange('integration_id', e.target.value)
                    onIntegrationChange(e.target.value)
                  }}
                >
                  <option value="">Select integration…</option>
                  {traefikIntegrations.map(i => (
                    <option key={i.id} value={i.id}>{i.name}</option>
                  ))}
                </select>
              </div>
            )}
            <div className="form-field">
              <div className="form-label">Domain</div>
              {traefikCerts.length === 0 ? (
                <div className="ssl-no-certs-msg">
                  {selectedIntegration
                    ? 'No certs discovered yet — run a sync in Settings → Integrations.'
                    : 'Select an integration to see available domains.'}
                </div>
              ) : (
                <select
                  className="form-input"
                  value={form.traefik_domain}
                  onChange={e => onChange('traefik_domain', e.target.value)}
                >
                  <option value="">Select domain…</option>
                  {traefikCerts.map(c => (
                    <option key={c.id} value={c.domain}>{c.domain}</option>
                  ))}
                </select>
              )}
            </div>
          </>
        )}

        {(form.type !== 'ssl' || form.ssl_source === 'standalone' || !hasTraefik) && (
          <div className="form-field">
            <div className="form-label">{form.type === 'ping' ? 'Host / IP' : 'URL'}</div>
            <input
              className="form-input"
              value={form.target}
              onChange={e => onChange('target', e.target.value)}
              placeholder={form.type === 'ping' ? 'e.g. 192.168.1.1' : 'https://example.com'}
            />
            {form.type === 'ssl' && form.ssl_source === 'standalone' && (
              <div className="ssl-standalone-warning">
                ⚠ Standalone SSL checks make a direct TLS connection. This may fail for
                services proxied through Traefik on the same host. Use for external URLs only.
              </div>
            )}
          </div>
        )}

        <div className="form-field">
          <div className="form-label">Interval (seconds)</div>
          <input
            className="form-input"
            type="number"
            min="30"
            value={form.interval_secs}
            onChange={e => onChange('interval_secs', e.target.value)}
          />
        </div>

        {form.type === 'url' && (
          <>
            <div className="form-field">
              <div className="form-label">Expected Status</div>
              <input
                className="form-input"
                type="number"
                value={form.expected_status}
                onChange={e => onChange('expected_status', e.target.value)}
                placeholder="200"
              />
            </div>
            <div className="form-field form-field-full">
              <label className="form-checkbox-label">
                <input
                  type="checkbox"
                  className="form-checkbox"
                  checked={form.skip_tls_verify === 'true'}
                  onChange={e => onChange('skip_tls_verify', e.target.checked ? 'true' : 'false')}
                />
                <span className="form-checkbox-text">
                  Accept self-signed certificates
                  <span className="form-checkbox-hint"> — skips TLS verification; use for internal services only</span>
                </span>
              </label>
            </div>
          </>
        )}

        {form.type === 'ssl' && (
          <>
            <div className="form-field">
              <div className="form-label">Warn Threshold (days)</div>
              <input
                className="form-input"
                type="number"
                min="1"
                value={form.ssl_warn_days}
                onChange={e => onChange('ssl_warn_days', e.target.value)}
                placeholder="30"
              />
            </div>
            <div className="form-field">
              <div className="form-label">Critical Threshold (days)</div>
              <input
                className="form-input"
                type="number"
                min="1"
                value={form.ssl_crit_days}
                onChange={e => onChange('ssl_crit_days', e.target.value)}
                placeholder="7"
              />
            </div>
          </>
        )}
      </div>
      {error && <div className="form-error">{error}</div>}
      <div className="form-actions">
        <button className="form-btn primary" onClick={onSubmit} disabled={submitting}>
          {submitting ? 'Saving…' : submitLabel}
        </button>
        <button className="form-btn secondary" onClick={onCancel}>
          Cancel
        </button>
        {extraAction}
      </div>
    </div>
  )
}
