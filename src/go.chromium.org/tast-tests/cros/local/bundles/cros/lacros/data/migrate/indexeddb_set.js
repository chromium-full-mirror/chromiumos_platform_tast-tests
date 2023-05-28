// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// JS script to set a userId => userEmail entry
// in someDataBase IndexedDB.
(userId, userEmail) => {
    return new Promise((resolve, reject) => {
        const req = window.indexedDB.open("someDataBase", 1);
        req.onerror = () => {
            console.error("Opening a database failed.");
            reject();
        }
        req.onupgradeneeded = e => {
            const db = e.target.result;
            const objectStore =
                db.createObjectStore("users", { keyPath: "id" });
            objectStore.transaction.onerror = () => {
                console.error("Creating an object store failed.");
                reject();
            }
            objectStore.transaction.oncomplete = () => {
                const userObjectStore = db.transaction("users", 'readwrite')
                    .objectStore("users");
                const req =
                    userObjectStore.add({ id: userId, email: userEmail });
                req.error = () => {
                    console.error("Adding an entry to database failed.");
                    reject();
                };
                req.onsuccess = () => resolve();
            }
        }
    })
}
