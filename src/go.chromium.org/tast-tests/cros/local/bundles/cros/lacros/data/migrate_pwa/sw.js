// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

const cacheName = "v1";

const channelName = "sw-message";

const cacheAssets = ["/migrate_pwa/pwa.html"];

console.log("SW: started!");

const channel = new BroadcastChannel(channelName);

self.addEventListener("install", (e) => {
  console.log("SW: Installing");

  e.waitUntil(
    caches
      .open(cacheName)
      .then((cache) => {
        console.log("SW: Caching files");
        cache.addAll(cacheAssets);
      })
      .catch((err) => {
        console.log("SW: Failed to open cache");
        console.error(err);
        channel.postMessage({ ok: false });
      })
      .then(() => {
        self.skipWaiting();
        console.log("SW: Caches added");
        channel.postMessage({ ok: true });
      })
      .catch((err) => {
        console.error("SW: Adding caches failed");
        console.error(err);
        channel.postMessage({ ok: false });
      })
  );
});

self.addEventListener("fetch", (e) => {
  console.log(`SW: Fetching ${e.request.url}`);
  e.respondWith(caches.match(e.request) || fetch(e.request));
});
