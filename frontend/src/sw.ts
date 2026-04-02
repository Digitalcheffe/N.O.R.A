import { precacheAndRoute } from 'workbox-precaching'

declare let self: ServiceWorkerGlobalScope

precacheAndRoute(self.__WB_MANIFEST)

self.addEventListener('push', (event) => {
  const data = event.data?.json() as Record<string, string> ?? {}
  const title = data['title'] ?? 'NORA'
  const options: NotificationOptions = {
    body: data['body'] ?? '',
    icon: '/icons/icon.svg',
    badge: '/icons/badge.svg',
    data: { url: data['url'] ?? '/' },
  }
  event.waitUntil(self.registration.showNotification(title, options))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  event.waitUntil(
    self.clients.openWindow((event.notification.data as { url?: string })?.url ?? '/')
  )
})
