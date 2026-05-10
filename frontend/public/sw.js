// Levelbrook portal service worker. Minimal cache-first shell so the /u/:token
// route loads instantly on repeat visits and survives a parking-lot signal
// dropout. Network is required for the actual upload — the SW deliberately
// does not cache POST responses.
const CACHE = 'lb-portal-v1';
const SHELL = ['/', '/index.html', '/manifest.webmanifest', '/icon-192.svg'];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))),
    ).then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  if (event.request.method !== 'GET') return; // never cache mutations
  const url = new URL(event.request.url);
  // Same-origin, navigation or static assets only.
  if (url.origin !== self.location.origin) return;
  event.respondWith(
    caches.match(event.request).then((hit) => {
      if (hit) return hit;
      return fetch(event.request).then((res) => {
        if (res.ok) {
          const copy = res.clone();
          caches.open(CACHE).then((c) => c.put(event.request, copy)).catch(() => {});
        }
        return res;
      }).catch(() => caches.match('/'));
    }),
  );
});
