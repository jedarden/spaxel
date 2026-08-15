// Spaxel Dashboard Service Worker
// Caches static shell for fast repeat loads, but NEVER caches live WebSocket or REST API data

const CACHE_NAME = 'spaxel-dashboard-v1';
const STATIC_CACHE = 'spaxel-static-v1';

// Static assets to cache - these are embedded via go:embed and change only on deployment
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/live.html',
  '/css/tokens.css',
  '/css/layout.css',
  '/css/home.css',
  '/css/troubleshoot.css',
  '/css/panels.css',
  '/css/timeline.css',
  '/css/notifications.css',
  '/css/apdetection.css',
  '/css/ble-panel.css',
  '/css/security.css',
  '/css/anomaly.css',
  '/css/sleep.css',
  '/css/floorplan.css',
  '/css/explainability.css',
  '/css/replay.css',
  '/css/scene.css',
  '/css/command-palette.css',
  '/css/ambient.css',
  '/css/guided-help.css',
  '/css/quick-actions.css',
  '/css/briefing.css',
  '/css/simulator.css',
  '/static/css/mobile.css',
  '/js/home-cards.js',
  '/js/onboard.js',
  '/js/troubleshoot.js',
  '/manifest.json'
];

// Install event - cache static shell
self.addEventListener('install', (event) => {
  console.log('[SW] Installing service worker and caching static shell');

  event.waitUntil(
    caches.open(STATIC_CACHE)
      .then((cache) => {
        console.log('[SW] Caching static assets');
        return cache.addAll(STATIC_ASSETS);
      })
      .then(() => {
        console.log('[SW] Static shell cached successfully');
        return self.skipWaiting(); // Activate immediately
      })
      .catch((error) => {
        console.error('[SW] Failed to cache static assets:', error);
      })
  );
});

// Activate event - clean up old caches
self.addEventListener('activate', (event) => {
  console.log('[SW] Activating service worker');

  event.waitUntil(
    caches.keys()
      .then((cacheNames) => {
        return Promise.all(
          cacheNames.map((cacheName) => {
            // Delete old caches if they exist
            if (cacheName !== STATIC_CACHE && cacheName !== CACHE_NAME) {
              console.log('[SW] Deleting old cache:', cacheName);
              return caches.delete(cacheName);
            }
          })
        );
      })
      .then(() => {
        console.log('[SW] Service worker activated');
        return self.clients.claim(); // Take control immediately
      })
  );
});

// Fetch event - handle requests
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  // NEVER cache WebSocket connections (ws:// or wss://)
  if (url.protocol === 'ws:' || url.protocol === 'wss:') {
    console.log('[SW] WebSocket connection - bypassing cache');
    return;
  }

  // NEVER cache REST API responses - presence data must always be live
  // This includes our API endpoints like /api/presence, /api/events, etc.
  if (url.pathname.startsWith('/api/') ||
      url.pathname.startsWith('/ws') ||
      url.pathname.startsWith('/stream')) {
    console.log('[SW] API/WebSocket endpoint - network only:', url.pathname);
    event.respondWith(
      fetch(event.request)
        .catch((error) => {
          console.error('[SW] Network request failed for API:', error);
          // Return a offline error for API failures
          return new Response(
            JSON.stringify({
              error: 'offline',
              message: 'Network unavailable - cannot fetch live data'
            }),
            {
              status: 503,
              statusText: 'Service Unavailable',
              headers: { 'Content-Type': 'application/json' }
            }
          );
        })
    );
    return;
  }

  // Cache-first strategy for static assets (HTML, CSS, JS)
  // These are embedded via go:embed and are safe to cache
  if (STATIC_ASSETS.some(asset => url.pathname.endsWith(asset) || url.pathname === asset)) {
    event.respondWith(
      caches.match(event.request)
        .then((cachedResponse) => {
          if (cachedResponse) {
            console.log('[SW] Serving from cache:', url.pathname);
            // Background update cache
            fetch(event.request).then((networkResponse) => {
              caches.open(STATIC_CACHE).then((cache) => {
                cache.put(event.request, networkResponse);
              });
            }).catch(() => {
              // Network error - serve from cache is fine
            });
            return cachedResponse;
          }

          // Not in cache - fetch from network
          console.log('[SW] Fetching from network:', url.pathname);
          return fetch(event.request)
            .then((networkResponse) => {
              // Cache the response for future use
              if (networkResponse && networkResponse.status === 200) {
                const responseToCache = networkResponse.clone();
                caches.open(STATIC_CACHE).then((cache) => {
                  cache.put(event.request, responseToCache);
                });
              }
              return networkResponse;
            })
            .catch((error) => {
              console.error('[SW] Network request failed:', error);
              // Return a basic offline page for HTML requests
              if (event.request.headers.get('accept')?.includes('text/html')) {
                return caches.match('/index.html') || new Response(
                  '<h1>Offline - Spaxel Dashboard</h1><p>Check your internet connection</p>',
                  { headers: { 'Content-Type': 'text/html' } }
                );
              }
              throw error;
            });
        })
    );
    return;
  }

  // Network-first for other resources (icons, etc.)
  event.respondWith(
    fetch(event.request)
      .then((networkResponse) => {
        // Cache successful responses
        if (networkResponse && networkResponse.status === 200) {
          const responseToCache = networkResponse.clone();
          caches.open(CACHE_NAME).then((cache) => {
            cache.put(event.request, responseToCache);
          });
        }
        return networkResponse;
      })
      .catch(() => {
        // Try cache as fallback
        return caches.match(event.request);
      })
  );
});

// Handle background sync for future enhancements (e.g., offline data queuing)
self.addEventListener('sync', (event) => {
  console.log('[SW] Background sync:', event.tag);
  // Future: Implement sync for offline data queuing
});

// Handle push notifications for future enhancements
self.addEventListener('push', (event) => {
  console.log('[SW] Push notification received');
  // Future: Implement push notifications for fall alerts, etc.
});

// Periodic background sync for updating cached content (if supported)
self.addEventListener('periodicsync', (event) => {
  if (event.tag === 'update-static-cache') {
    event.waitUntil(
      caches.open(STATIC_CACHE).then((cache) => {
        return cache.addAll(STATIC_ASSETS);
      })
    );
  }
});
