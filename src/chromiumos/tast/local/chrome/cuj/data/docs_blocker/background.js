// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Blocks Docs network access to force it to work in offline mode.
// Testing users should have "Docs offline" extension installed and
// offline enabled in Docs settings.

// Whether to block Docs network access.
let offline = false;

function onBeforeRequestListener(details) {
  return {cancel: offline && details.url.indexOf("docs") != -1};
}

function ForceDocsOffline(offlineRequested) {
  if (offline == offlineRequested)
    return;

  offline = offlineRequested;

  if (offlineRequested) {
    chrome.webRequest.onBeforeRequest.addListener(
        onBeforeRequestListener, {urls: ['*://*/*']}, ['blocking']);
  } else {
    chrome.webRequest.onBeforeRequest.removeListener(onBeforeRequestListener);
  }
}
