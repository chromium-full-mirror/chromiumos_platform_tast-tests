// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

function setTitle(tabId, capture) {
  let title = "Screen capture allowed";
  if (!capture) {
      if (chrome.runtime.lastError) {
          title = chrome.runtime.lastError.message;
      } else {
          title = "Unknown error";
      }
  }

  chrome.scripting.executeScript({
    target: { tabId: tabId },
    func: (newTitle) => {
      document.title = newTitle;
    },
    args: [title]
  });
}

chrome.commands.onCommand.addListener((command) => {
  if (command === 'takeScreenshot') {
    chrome.tabs.query({active: true, currentWindow: true}, (tabs) => {
      const activeTab = tabs[0];
      const tabId = activeTab.id;
      chrome.tabs.sendMessage(tabs[0].id, {text: 'title'}, (method) => {
          if (method === 'captureVisibleTab') {
            chrome.tabs.captureVisibleTab((img) => {
              setTitle(tabId, img);
            });
          } else if (method === 'tabCapture') {
            chrome.tabCapture.capture({video: true}, (stream) => {
              setTitle(tabId, stream);
            });
          } else if (method === 'desktopCapture') {
            chrome.desktopCapture.chooseDesktopMedia(
              ['screen', 'window', 'tab'], (streamId) => {
                setTitle(tabId, streamId);
              });
          }
      });
    });
  }
});
