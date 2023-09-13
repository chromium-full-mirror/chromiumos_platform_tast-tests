// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// JS script to check a userId => userEmail entry
// in someDataBase IndexedDB.
(userId, userEmail) => {
    return new Promise((resolve, reject) => {
        const req = window.indexedDB.open("someDataBase");
        req.onerror = () => {
            console.error("Opening database 'someDataBase' failed.");
            reject();
        }
        req.onsuccess = e => {
            try {
                const db = e.target.result;
                const trans = db.transaction("users");
                trans.onerror = () => {
                    console.error("Transaction on users failed.");
                    reject();
                }
                const objStore = trans.objectStore("users");
                const req = objStore.get(userId);
                req.onsuccess = () => {
                    if (req.result.email != userEmail) {
                        console.error("userEmail != " + req.result.email);
                        reject();
                    } else {
                        console.log("Successfully retrieved email "
                            + req.result.email);
                        resolve();
                    }
                }
                req.onerror = () => {
                    console.error("Failed to get user with id: "
                        + userId);
                    reject();
                }
            } catch (err) {
                // Calls like `db.transaction` can throw an exception
                // synchronously that is not caught by `onerror` handler
                // so make sure to catch them and call `reject`. Specifically
                // this can happen if the store 'users' do not exist.
                console.error(err);
                reject();
            }
        }
    })
}
