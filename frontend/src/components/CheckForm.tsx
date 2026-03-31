import type { ReactNode } from 'react'
import type { CheckType, InfraIntegration, TraefikCert } from '../api/types'
import type { FormFields } from './checkFormHelpers'
import './CheckForm.css'

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
