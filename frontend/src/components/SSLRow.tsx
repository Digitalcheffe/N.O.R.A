import type { SSLCert } from '../api/types'

function daysClass(days: number): string {
  if (days <= 10) return 'ssl-days crit'
  if (days <= 30) return 'ssl-days warn'
  return 'ssl-days good'
}

function formatSSLDate(dateStr: string): string {
  if (!dateStr) return ''
  const d = new Date(dateStr + 'T00:00:00Z')
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' })
}

interface Props {
  cert: SSLCert
}

export function SSLRow({ cert }: Props) {
  return (
    <div className="ssl-row">
      <div className={daysClass(cert.days_remaining)}>
        {cert.days_remaining}d
      </div>
      <div className="ssl-domain">{cert.domain}</div>
      <div className="ssl-date">{formatSSLDate(cert.expires_at)}</div>
    </div>
  )
}
