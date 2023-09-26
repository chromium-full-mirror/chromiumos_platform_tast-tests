// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// The function returns a promise that registers a service worker and resolves
// once caching is completed.
() => {
    return new Promise((resolve, reject) => {
        if (!("serviceWorker" in navigator)) {
            console.error("Service Worker is not supported.");
            reject();
        }

        navigator.serviceWorker
            .register("/migrate_pwa/sw.js")
            .then(reg => {
                console.log("Service Worker registered.");
            })
            .catch(err => {
                console.error(`Failed to register: ${err}`);
                reject();
            });

        const channel = new BroadcastChannel("sw-message");
        channel.addEventListener("message", e => {
            if (e.data.ok) {
                resolve();
            } else {
                console.error("Caching failed at some stage.");
                reject();
            }
        })
    })
}
