// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
'use strict';

// FpsCounter calculates a simple moving average of `NUM_FRAMES_TO_AVERAGE`
// frames.
class FpsCounter {
  static NUM_FRAMES_TO_AVERAGE = 16;

  constructor(){
    // Total time spent for last N frames. Initialling it to
    // N(number of frames to average) ensures we do not divide by zero
    // as we calculate the average fps.
    this.totalTime_ = FpsCounter.NUM_FRAMES_TO_AVERAGE;

    // Elapsed time for last N frames.
    this.frameTimes_ = [];

    // Where to record next elapsed time in frameTimes_.
    this.frameTimesIndex_ = 0;

    this.lastRecordedTime_ = 0.0;

    // Initialize the FPS elapsed time. Since totalTime_ is initialized to
    // N (number of frames to average), as we calculate the average of first N
    // frames, we subtract N from the totalTime_, normalizing the average.
    for (let index = 0; index < FpsCounter.NUM_FRAMES_TO_AVERAGE; ++index) {
      this.frameTimes_[index] = 1.0;
    }

    this.averageFps_ = 0;
  }

  // Updates the fps measurement. Call it after rendering each frame.
  update(){
    const elapsedTime = this.getElapsedTime()

    // Keep the total time and total active time for the last N frames.
    this.totalTime_ += elapsedTime - this.frameTimes_[this.frameTimesIndex_];

    // Save off the elapsed time for this frame so we can subtract it later.
    this.frameTimes_[this.frameTimesIndex_] = elapsedTime;

    this.frameTimesIndex_++;
    this.frameTimesIndex_ %= FpsCounter.NUM_FRAMES_TO_AVERAGE;

    // We do floor(averageFps + 0.5) to rounded the value to nearest whole
    // number.
    this.averageFps_ = Math.floor(
      (FpsCounter.NUM_FRAMES_TO_AVERAGE / this.totalTime_) + 0.5);
  }

  averageFps(){
    return this.averageFps_;
  }

  getElapsedTime(){
    const now = this.getCurrentTimeInSecond();
    let elapsedTime;

    if(this.lastRecordedTime_ == 0.0) {
      elapsedTime = 0.0;
    } else {
      elapsedTime = now - this.lastRecordedTime_;
    }

    this.lastRecordedTime_ = now;
    return elapsedTime;
  }

  getCurrentTimeInSecond(){
    return (new Date()).getTime() * 0.001;
  }
}

let pixel_width;
let pixel_height;
let deg = 0;
let fpsCounter = new FpsCounter();

function init2D() {
  const canvas = document.querySelector('canvas');
  const container = document.querySelector('#container');

  const setSizeAndRotation = () => {
    const angle = screen.orientation.angle % 360;
    const dpr = devicePixelRatio;
    const dp_width = container.clientWidth;
    const dp_height = container.clientHeight;
    pixel_width = Math.round(dp_width * dpr);
    pixel_height = Math.round(dp_height * dpr);

    if (angle % 180 == 90) {
      canvas.style.width = `${dp_height}px`;
      canvas.style.height = `${dp_width}px`;
      const tmp = pixel_height;
      pixel_height = pixel_width;
      pixel_width = tmp;
    } else {
      canvas.style.width = `${dp_width}px`;
      canvas.style.height = `${dp_height}px`;
    }
    canvas.width  = pixel_width;
    canvas.height = pixel_height;

    canvas.style.transform = `rotateZ(${angle}deg)`;
    switch (angle) {
      case 0:
        canvas.style.left = "0px";
        canvas.style.top = "0px";
        break;
      case 90:
        canvas.style.left = `${dp_width}px`;
        canvas.style.top = "0px";
        break;
      case 180:
        canvas.style.left = `${dp_width}px`;
        canvas.style.top = `${dp_height}px`;
        break;
      case 270:
        canvas.style.left = "0px";
        canvas.style.top = `${dp_height}px`;
        break;
    }

    console.log("update1:" + angle + ", size=" + dp_width + "x" + dp_height +
                " angle=" + screen.orientation.angle);
  };
  screen.orientation.addEventListener('change', setSizeAndRotation);
  window.addEventListener('resize',  setSizeAndRotation);
  setSizeAndRotation();
  draw();
}

function draw() {
  const angle = screen.orientation.angle % 360;
  const dpr = devicePixelRatio;
  const container = document.querySelector('#container');
  const dp_width = container.clientWidth;
  const dp_height = container.clientHeight;

  const canvas = document.querySelector('canvas');
  const context = canvas.getContext('2d', {desynchronized: true, alpha: false});

  context.fillStyle = 'rgb(255,255,0)';
  context.fillRect(0, 0, pixel_width, pixel_height);
  context.strokeStyle = 'rgb(255,0,0)';
  context.strokeRect(0, 0, pixel_width, pixel_height);

  // Text
  context.fillStyle = 'rgb(255,255,255)';
  context.font = "40px Arial";
  const text = `Pixel_size=${pixel_width}x${pixel_height} \
dp_size=${dp_width}x${dp_height} dpr=${dpr} angle=${angle} \
average_fps=${fpsCounter.averageFps()}`;
  context.fillText(text, 10, 50);
  context.strokeStyle = 'rgb(0,0,0)';
  context.strokeText(text, 10, 50);

  context.save();
  context.translate(300, 300);
  context.rotate(deg);
  deg += 0.02;
  context.fillStyle = 'rgb(255,255,255)';
  context.fillRect(-150, -150, 300, 300);
  context.restore();

  fpsCounter.update();

  // don't use requestAnimationFrame
  setTimeout(draw, 16);
}

function getCurrentAverageFps() {
  return fpsCounter.averageFps();
}
