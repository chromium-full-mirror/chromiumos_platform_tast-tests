// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// JS script to check a userId => userEmail entry
// in someDataBase IndexedDB.
(userId, userEmail) => {
    return new Promise((resolve, reject) => {
        const req = window.indexedDB.open("someDataBase");
        req.onerror = () => {
            console.log("Opening database 'someDataBase' failed.");
            reject();
        }
        req.onsuccess = e => {
            const db = e.target.result;
            const req =
                db.transaction("users").objectStore("users").get(userId);
            req.onsuccess = () => {
                if (req.result.email == userEmail) {
                    resolve();
                } else {
                    console.error("userEmail != " + req.result.email);
                    reject();
                }
            }
            req.onerror = () => {
                console.error("Failed to get user with id: " + userId);
                reject();
            }
        }
    })
}
