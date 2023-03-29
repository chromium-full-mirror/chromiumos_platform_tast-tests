// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

async (seconds) => {
  const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));

  // Meet can maintain two <video>s, one with effects applied and one without.
  // The second only appears once effects are enabled, then stays around.
  // The one not being used is simply hidden with `display: none` CSS, so pick
  // the one which is currently visible.
  const videos = document.querySelectorAll('video');
  const video = Array.prototype.find.call(videos, (video) => video.style.display
      !== 'none');

  let frames = 0;
  let abort = false;

  // Count frames in parallel.
  const countFrames = () => {
    frames += 1;
    if (!abort) {
      video.requestVideoFrameCallback(countFrames);
    }
  }
  video.requestVideoFrameCallback(countFrames);

  const data = [];
  const beginFrames = frames;
  const beginTime = Date.now();
  for (let i = 0; i < seconds; i += 1) {
    const oldFrames = frames;
    const oldTime = Date.now();
    await sleep(1000);
    const fps = (frames - oldFrames) / (Date.now() - oldTime) * 1000;
    data.push(fps);
  }
  const average = ((frames - beginFrames) / (Date.now() - beginTime) *
      1000);

  // Stop counting frames in parallel.
  abort = true;

  return {
    average,
    data
  };
}
