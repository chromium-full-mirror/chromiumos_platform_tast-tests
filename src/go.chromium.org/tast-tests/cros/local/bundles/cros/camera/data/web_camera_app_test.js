// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

let scriptReady = false;
let isTestDone = false;
let enumerateDevicesError = '';
const globalErrors = [];
const results = [];
const logs = [];

/**
 * Add logs.
 * @param {string} msg log message
 */
function addLog(msg) {
  const currentTime = new Date().toISOString();
  logs.push(`[${currentTime}] ${msg}`);
}

/**
 * Wait for id to display or not display.
 * @param {string} id tracked element.
 * @param {boolean} display true for show and false for hide.
 * @return {!Promise<void>} a Promise.
 */
async function waitForElementDisplay(id, display) {
  await waitFor(() => {
    const element = document.getElementById(id);
    const displayPropertyNotNone = getComputedStyle(element).display !== 'none';
    return (display === displayPropertyNotNone);
  });
}

/**
 * Wait for init to complete.
 * @return {!Promise<void>} a Promise.
 */
async function waitForInit() {
  await waitFor(() => window.app.initDone);
}

async function waitFor(condition, interval = 20) {
  return new Promise((resolve) => {
    (function poll() {
      if (condition() || globalErrors.length !== 0) {
        return resolve();
      }
      setTimeout(poll, interval);
    })();
  });
}

/**
 * Throws an error if `globalErrors` is not empty.
 */
function throwIfError(msg) {
  if (globalErrors.length !== 0) {
    throw new Error(msg + String(globalErrors[0]));
  }
}

/**
 * Testing code.
 * @param {number} durationSec Time for waiting.
 */
async function testWebCameraApp(durationSec) {
  addLog('testWebCameraApp');
  try {
    await waitForInit();
    throwIfError("Fail to wait for Init done: ");
    const resolutionOptions = ['640 480', '1280 720'];
    const frameRateOptions = [30, 2];
    const changeCameraSelection =
      document.getElementById('change-camera-selection');
    const frameRateInput = document.getElementById('frame-rate-input');
    const cameraResolution = document.getElementById('camera-resolution');
    const cameraOptions = changeCameraSelection.children;
    for (const cameraOption of cameraOptions) {
      changeCameraSelection.value = cameraOption.value;
      changeCameraSelection.dispatchEvent(new Event('change'));
      await waitForElementDisplay('web-loader', false);
      throwIfError("Fail to change device: ");
      for (const frameRateOption of frameRateOptions) {
        for (const resolutionOption of resolutionOptions) {
          frameRateInput.value = frameRateOption;
          frameRateInput.dispatchEvent(new Event('change'));
          await waitForElementDisplay('web-loader', false);
          throwIfError("Fail to change frame rate: ");
          cameraResolution.value = resolutionOption
          cameraResolution.dispatchEvent(new Event('change'));
          await waitForElementDisplay('web-loader', false);
          throwIfError("Fail to change resolution: ");

          document.getElementById('take-snapshot-button').click();
          throwIfError("Fail to take a snapshot: ");

          document.getElementById('start-button').click();
          await waitForElementDisplay('video-output', false);
          throwIfError("Fail to start recording: ");
          await new Promise((r) => setTimeout(r, durationSec * 1000));

          document.getElementById('stop-button').click();
          await waitForElementDisplay('video-output', true);
          throwIfError("Fail to stop recording: ");
        }
      }
    }
  } finally {
    reportTestDone();
  }
}

/**
 * Mark as done when the test is complete.
 */
function reportTestDone() {
  isTestDone = true;
}

/**
 * Get the results.
 * @return {string} result.
 */
function getResults() {
  return results;
}

/**
 * Get the logs.
 * @return {string} log.
 */
function getLogs() {
  return logs;
}

/**
 * check if a video input exists.
 * @return {boolean} the video input is found or not.
 */
async function checkVideoInput() {
  let isVideoInputFound = false;
  addLog('checkVideoInput');
  try {
    const devices = await navigator.mediaDevices.enumerateDevices();
    isVideoInputFound = devices.some((dev) => dev.kind == 'videoinput');
  } catch (error) {
    gotEnumerateDevicesError(error);
  }
  return isVideoInputFound;
}

/**
 * check if getting enumerate devices error.
 * @param {!Error} error message.
 */
function gotEnumerateDevicesError(error) {
  enumerateDevicesError = error.toString();
}

window.addEventListener("error", (event) => {
  globalErrors.push(event.error);
});

window.addEventListener("unhandledrejection", (event) => {
  globalErrors.push(event.reason);
});

scriptReady = true;
