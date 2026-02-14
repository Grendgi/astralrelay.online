/* Service Worker for Web Push */
self.addEventListener('push', function (event) {
  if (!event.data) return
  let data = { title: 'Мессенджер', body: 'Новое сообщение' }
  try {
    const j = event.data.json()
    if (j.title) data.title = j.title
    if (j.body) data.body = j.body
  } catch (_) {}
  event.waitUntil(
    self.registration.showNotification(data.title, {
      body: data.body,
      icon: '/favicon.ico',
      tag: 'messenger',
      requireInteraction: false,
    })
  )
})

self.addEventListener('notificationclick', function (event) {
  event.notification.close()
  event.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true }).then(function (list) {
      if (list.length) {
        list[0].focus()
      } else if (clients.openWindow) {
        clients.openWindow('/')
      }
    })
  )
})
