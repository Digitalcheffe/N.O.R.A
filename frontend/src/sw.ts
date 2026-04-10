import { precacheAndRoute, createHandlerBoundToURL } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'

declare let self: ServiceWorkerGlobalScope

precacheAndRoute(self.__WB_MANIFEST)

// SPA navigation fallback — any navigation request (clicking a link, typing a URL)
// that isn't a precached asset gets served index.html so React Router handles it.
registerRoute(new NavigationRoute(createHandlerBoundToURL('index.html')))

self.addEventListener('push', (event) => {
  const data = event.data?.json() as Record<string, string> ?? {}
  const title = data['title'] ?? 'NORA'
  const options: NotificationOptions = {
    body: data['body'] ?? '',
    icon: '/icons/icon-192.png',
    badge: '/icons/badge-72.png',
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
