// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

onCreated = function(w) {};

chrome.runtime.onInstalled.addListener(() => {
  chrome.windows.create({
    url: "scan.html",
    state: chrome.windows.WindowState.MAXIMIZED,
    focused: true,
  }, onCreated);
});
