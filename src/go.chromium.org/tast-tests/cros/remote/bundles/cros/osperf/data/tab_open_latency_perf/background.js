// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Open the benchmark UI when the extension icon is clicked
// for manual invocation.
chrome.action.onClicked.addListener((tab) => {
  chrome.tabs.create({'url': 'bench.html'});
});

function setIcon() {
  const size = 16;
  const r = size / 4;
  const d = size / 2;
  const cx = size / 2;
  const cy = size / 2;
  const canvas = new OffscreenCanvas(size, size);
  const ctx = canvas.getContext('2d');

  // Background color (Chromium blue)
  ctx.fillStyle = '#2f78e8';
  ctx.fillRect(0, 0, size, size);

  // Center circle
  ctx.beginPath();
  ctx.strokeWidth = '1px';
  ctx.strokeStyle = '#ffffff';
  ctx.arc(size / 2, size / 2, r, 0, 2 * Math.PI);
  ctx.stroke();

  // Lines
  const drawLine = (t) => {
    // Note: angle t is clockwise by default
    ctx.beginPath();
    const x = cx + r * Math.cos(t);
    const y = cy + r * Math.sin(t);
    ctx.moveTo(x, y);
    const u = t + Math.PI / 2;
    ctx.lineTo(x + d * Math.cos(u), y + d * Math.sin(u));
    ctx.stroke();
  };
  drawLine(Math.PI * 1);
  drawLine(Math.PI * 1 + Math.PI * 2 / 3);
  drawLine(Math.PI * 1 - Math.PI * 2 / 3);
  const imageData = ctx.getImageData(0, 0, size, size);
  chrome.action.setIcon(
      {imageData: imageData},
      () => {
          // Do nothing
      });
}
setIcon();
