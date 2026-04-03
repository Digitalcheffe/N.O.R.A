// Web Push requires HTTPS. In local dev, use localhost (not 192.168.x.x).
// For LAN access with push, use a reverse proxy with a real cert (e.g., Traefik + Let's Encrypt).

import { useState, useEffect } from 'react'
import { push } from '../api/client'

function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4)
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
  const rawData = window.atob(base64)
  const buffer = new ArrayBuffer(rawData.length)
  const output = new Uint8Array(buffer)
  for (let i = 0; i < rawData.length; i++) {
    output[i] = rawData.charCodeAt(i)
  }
  return output
}

export interface PushSubscriptionState {
  isSupported: boolean
  isSubscribed: boolean
  isLoading: boolean
  subscribe: () => Promise<void>
  unsubscribe: () => Promise<void>
}

export function usePushSubscription(): PushSubscriptionState {
  const isSupported = typeof window !== 'undefined' &&
    'PushManager' in window &&
    'serviceWorker' in navigator

  const [isSubscribed, setIsSubscribed] = useState(false)
  const [isLoading, setIsLoading] = useState(isSupported)

  useEffect(() => {
    if (!isSupported) return

    let cancelled = false
    navigator.serviceWorker.ready
      .then(reg => reg.pushManager.getSubscription())
      .then(sub => { if (!cancelled) setIsSubscribed(sub !== null) })
      .catch(() => { /* ignore — treat as unsubscribed */ })
      .finally(() => { if (!cancelled) setIsLoading(false) })

    return () => { cancelled = true }
  }, [isSupported])

  const subscribe = async () => {
    setIsLoading(true)
    try {
      const { public_key } = await push.vapidPublicKey()
      const reg = await navigator.serviceWorker.ready
      const subscription = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      })
      const json = subscription.toJSON()
      await push.subscribe({
        endpoint: subscription.endpoint,
        keys: {
          p256dh: json.keys?.['p256dh'] ?? '',
          auth: json.keys?.['auth'] ?? '',
        },
      })
      setIsSubscribed(true)
    } finally {
      setIsLoading(false)
    }
  }

  const unsubscribe = async () => {
    setIsLoading(true)
    try {
      const reg = await navigator.serviceWorker.ready
      const subscription = await reg.pushManager.getSubscription()
      if (subscription) {
        // Best-effort backend removal — don't let a 404 block browser cleanup.
        try {
          await push.unsubscribe({ endpoint: subscription.endpoint })
        } catch {
          // Subscription may already be gone from the server; continue cleanup.
        }
        await subscription.unsubscribe()
      }
      setIsSubscribed(false)
    } finally {
      setIsLoading(false)
    }
  }

  return { isSupported, isSubscribed, isLoading, subscribe, unsubscribe }
}
