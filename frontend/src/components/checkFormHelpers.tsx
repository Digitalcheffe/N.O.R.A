import type { ReactNode } from 'react'
import type { MonitorCheck, CreateCheckInput, CheckType, SSLSource, DNSRecordType } from '../api/types'

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
  skip_tls_verify: string   // 'true' | 'false'
  dns_record_type: DNSRecordType
  dns_expected_value: string
  dns_resolver: string
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
  dns_record_type: 'A',
  dns_expected_value: '',
  dns_resolver: '',
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

  if (form.type === 'dns' && !form.target.trim()) return 'Hostname is required'

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
    dns_record_type: (check.dns_record_type ?? 'A') as DNSRecordType,
    dns_expected_value: check.dns_expected_value ?? '',
    dns_resolver: check.dns_resolver ?? '',
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
  if (form.type === 'dns') {
    input.dns_record_type = form.dns_record_type
    if (form.dns_expected_value.trim()) {
      input.dns_expected_value = form.dns_expected_value.trim()
    }
    if (form.dns_resolver.trim()) {
      input.dns_resolver = form.dns_resolver.trim()
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
    if (check.type === 'dns') {
      if (r.error) return (
        <div className="check-result-grid">
          <span className="check-result-label">Error</span>
          <span className="check-result-value check-result-error">{String(r.error)}</span>
        </div>
      )
      const records = Array.isArray(r.records) ? (r.records as string[]) : []
      return (
        <div className="check-result-grid">
          <span className="check-result-label">Record type</span>
          <span className="check-result-value">{r.record_type != null ? String(r.record_type) : '—'}</span>
          <span className="check-result-label">Resolved</span>
          <span className="check-result-value">{records.length > 0 ? records.join(', ') : '—'}</span>
          {check.dns_expected_value && <>
            <span className="check-result-label">Baseline</span>
            <span className="check-result-value">{check.dns_expected_value}</span>
          </>}
          <span className="check-result-label">Latency</span>
          <span className="check-result-value">{r.latency_ms != null ? `${r.latency_ms}ms` : '—'}</span>
        </div>
      )
    }
  } catch { /* fall through */ }
  return <span className="check-result-empty">{check.last_result}</span>
}
