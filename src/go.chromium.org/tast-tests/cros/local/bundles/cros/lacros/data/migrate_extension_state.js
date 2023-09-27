// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

const storageSyncValue = "syncValue1234";

function setStorageSync() {
  return new Promise((resolve, reject) => {
    chrome.storage.sync
      .set({ key: storageSyncValue })
      .then(() => resolve())
      .catch((err) => reject(err));
  });
}

function verifyStorageSync() {
  return new Promise((resolve, reject) => {
    chrome.storage.sync
      .get(["key"])
      .then((result) => {
        if (result.key == storageSyncValue) resolve();
        else
          reject(new Error(`wanted ${storageSyncValue} but got ${result.key}`));
      })
      .catch((err) => reject(err));
  });
}

const storageLocalValue = "localValue1234";

function setStorageLocal() {
  return new Promise((resolve, reject) => {
    chrome.storage.local
      .set({ key: storageLocalValue })
      .then(() => resolve())
      .catch((err) => {
        reject(err);
      });
  });
}

function verifyStorageLocal() {
  return new Promise((resolve, reject) => {
    chrome.storage.local
      .get(["key"])
      .then((result) => {
        if (result.key == storageLocalValue) resolve();
        else
          reject(
            new Error(`wanted ${storageLocalValue} but got ${result.key}`)
          );
      })
      .catch((err) => {
        reject(err);
      });
  });
}
