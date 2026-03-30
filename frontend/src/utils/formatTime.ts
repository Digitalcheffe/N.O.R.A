/**
 * Shared timestamp formatting utility for NORA.
 *
 * Display rules (local timezone):
 *   - Today           → time only          e.g. "2:47 PM"
 *   - Prior day, same year → date + time   e.g. "3/29 2:47 PM"
 *   - Prior year      → full date + time   e.g. "12/15/2025 2:47 PM"
 *
 * Never returns relative strings ("Yesterday", "2 hours ago", "just now").
 */
export function formatEventTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'

  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const timePart = d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', hour12: true })

  if (d >= startOfToday) {
    return timePart
  }

  if (d.getFullYear() === now.getFullYear()) {
    const month = d.getMonth() + 1
    const day = d.getDate()
    return `${month}/${day} ${timePart}`
  }

  const month = d.getMonth() + 1
  const day = d.getDate()
  const year = d.getFullYear()
  return `${month}/${day}/${year} ${timePart}`
}
