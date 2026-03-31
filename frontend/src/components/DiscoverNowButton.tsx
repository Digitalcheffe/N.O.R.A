import { useState } from 'react'
import { infrastructure as infraApi } from '../api/client'

interface Props {
  /** The component type (e.g. "proxmox", "docker_engine") — for display context */
  entityType: string
  /** The infrastructure component ID to discover */
  entityId: string
  /** Called after a successful discover so the parent can reload its data */
  onSuccess?: () => void
}

export function DiscoverNowButton({ entityId, onSuccess }: Props) {
  const [discovering, setDiscovering] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleClick() {
    if (discovering) return
    setDiscovering(true)
    setError(null)
    try {
      await infraApi.discover(entityId)
      onSuccess?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Discover failed')
    } finally {
      setDiscovering(false)
    }
  }

  return (
    <>
      <button
        className="dpl-discover-btn"
        onClick={() => void handleClick()}
        disabled={discovering}
      >
        {discovering ? 'Discovering…' : 'Discover Now'}
      </button>
      {error && (
        <span className="dpl-discover-error">{error}</span>
      )}
    </>
  )
}
