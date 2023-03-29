// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Because Zoom renders the preview itself into a canvas from inside wasm code
// running inside worker threads, it's very hard to inspect directly.
//
// Instead our strategy is to capture a MediaStream from the canvas, which lets
// us receive new frames as they are drawn. Because Zoom draws on its own clock
// regardless of the actual frame rate, we then need to compare frames to
// detect if the canvas has actually changed, which gives us a real frame rate
// as observed by the user.
//
// To avoid impacting the frame rate, we perform this inside our own worker
// thread.

async (seconds) => {
  const canvas = document.querySelector('canvas#multi-view-video');
  const stream = canvas.captureStream();
  const track = stream.getVideoTracks()[0];
  const processor = new MediaStreamTrackProcessor({ track });

  // Find rectangle within canvas containing video feed.
  // Round to even dimensions for 4:2:0 compatibility.
  const frame = document.querySelector(
    '.speaker-active-container__video-frame');
  const frameRect = frame.getBoundingClientRect();
  const rect = {
    x: Math.floor(frameRect.x / 2) * 2,
    y: Math.floor(frameRect.y / 2) * 2,
    width: Math.floor(frameRect.width / 2) * 2,
    height: Math.floor(frameRect.height / 2) * 2
  };

  // This function is serialized into a string and executed inside a worker
  // thread. Basically this means its parameters and return value must go via
  // messages instead, and it can't access any closure variables, so all
  // parameters must be explicitly passed in.
  const workerFunction = async () => {
    const arrayEquals = (a, b, step = 1) => {
      if (a.length !== b.length) {
        return false;
      }
      for (let i = a.length; -1 < i; i -= step) {
        if ((a[i] !== b[i])) return false;
      }
      return true;
    }

    const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));

    // Wait to receive parameters via message from main thread.
    const {
      readable,
      rect,
      seconds
    } = await (() => new Promise((resolve) => {
      this.onmessage = (event) => resolve(event.data);
    }))();

    let frames = 0;
    let abort = false;

    // Count frames in parallel.
    (async () => {
      const reader = readable.getReader();
      let prevBuffer = null;
      let buffer = null;
      while (!abort) {
        const { value: frame, done } = await reader.read();
        if (done) break;
        if (!prevBuffer || !buffer) {
          // Allocate buffers on first frame, then reuse them.
          // XXX Assumes window is never resized.
          const size = frame.allocationSize({ rect }) / 4;
          prevBuffer = new Uint32Array(size);
          buffer = new Uint32Array(size);
        }
        frame.copyTo(buffer, { rect });
        frame.close();
        // Roughly compare frames by sampling bytes,
        // significantly faster and works just as well.
        if (!arrayEquals(buffer, prevBuffer, 8)) {
          frames += 1;
          [prevBuffer, buffer] = [buffer, prevBuffer];
        }
      }
      reader.releaseLock();
    })();

    const data = [];
    const beginFrames = frames;
    const beginTime = Date.now();
    for (let i = 0; i < seconds; i += 1) {
      const oldFrames = frames;
      const oldTime = Date.now();
      await sleep(1000);
      const fps = (frames - oldFrames) / (Date.now() - oldTime) * 1000;
      data.push(fps);
      console.log('fps', fps.toFixed(3));
    }
    const average = ((frames - beginFrames) / (Date.now() - beginTime) *
        1000);
    console.log('average fps', average.toFixed(3));

    // Stop counting frames in parallel.
    abort = true;

    this.postMessage({
      average,
      data
    });
  };

  // Wait to receive result via message from worker thread.
  const result = await (() => new Promise((resolve, reject) => {
    const workerBlob = new Blob([`(${workerFunction})()`],
      { type: 'application/javascript' });
    const worker = new Worker(URL.createObjectURL(workerBlob));
    worker.onerror = (event) => reject('worker error');
    worker.onmessage = (event) => resolve(event.data);
    // Send parameters via message to worker thread.
    // Ownership of processor.readable is transferred to the worker thread.
    worker.postMessage({
      readable: processor.readable,
      rect,
      seconds
    }, [processor.readable]);
  }))();

  track.stop();

  return result;
}
