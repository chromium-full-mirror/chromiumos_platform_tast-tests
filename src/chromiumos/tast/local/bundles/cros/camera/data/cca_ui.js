// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

(async function() {

/**
 * Imports js modules from CCA source.
 * @param {string} path Import path related to CCA js directory.
 * @return {!Promise<!Module>} Resolves to module in cca.
 */
function ccaImport(path) {
  return import(`/js/${path}`);
}

const state = await ccaImport('state.js');
const localStorage = await ccaImport('models/local_storage.js');
const {DeviceOperator} = await ccaImport('mojo/device_operator.js');
const {ChromeHelper} = await ccaImport('mojo/chrome_helper.js');
const {Facing} = await ccaImport('type.js');
const {windowController} = await ccaImport('window_controller.js');

/**
 * @typedef {chrome.app.window.AppWindow} AppWindow
 */

class LegacyVCDError extends Error {
  /**
   * @param {string=} message
   * @public
   */
  constructor(
      message =
          'Call to unsupported mojo operation on legacy VCD implementation.') {
    super(message);
    this.name = this.constructor.name;
  }
};

/**
 * @typedef {{
 *   width: number,
 *   height: number,
 * }}
 */
var Resolution;

// Note: use a named class declaration and refer the class instance via the
// name here, instead of using anonymous class and referring it via "this",
// in order to make it simpler to call methods via Chrome Devtools Protocol.
window.Tast = class Tast {
  static get previewVideo() {
    return document.querySelector('#preview-video');
  }

  static getState(s) {
    return state.get(s);
  }

  /**
   * @param {number} ms
   * @return {!Promise}
   */
  static sleep(ms) {
    return new Promise((resolve) => {
      setTimeout(resolve, ms);
    });
  }

  /**
   * @return {string}
   */
  static getStyle(selector, attribute) {
    const element = document.querySelector(selector);
    if (element === null) {
      return "";
    }
    const style = window.getComputedStyle(element);
    return style.getPropertyValue(attribute);
  }

  /**
   * Returns whether the target HTML element is visible.
   * @param {string} selector Selector for the target element.
   * @return {boolean}
   */
  static isVisible(selector) {
    const element = document.querySelector(selector);
    if (element === null) {
      return false;
    }
    return Tast.isVisibleElement(element);
  }

  /**
   * Returns whether the target HTML element is visible.
   * @param {!HTMLElement} element
   * @return {boolean}
   */
  static isVisibleElement(element) {
    const style = window.getComputedStyle(element);
    return style.visibility !== 'hidden' && element.getClientRects().length > 0;
  }

  /**
   * Triggers click event on the target HTML element that specified by
   * |selector|. If more than one element matched the selector, it will
   * trigger the first one whose display property is non-null.
   * @param {string} selector Selector for the target element.
   */
  static click(selector) {
    const element = Array.from(document.querySelectorAll(selector))
                        .find(Tast.isVisibleElement);
    if (!element) {
      throw new Error('No visible element: ', selector);
    }
    element.click();
  }

  /**
   * Switches to specific camera mode.
   * @param {string} mode The target mode which we expects to switch to.
   * @throws {Error} Throws error if there is no button found for given |mode|.
   */
  static switchMode(mode) {
    Tast.click(`.mode-item>input[data-mode="${mode}"]`);
  }

  /**
   * Gets whether portrait mode is supported by current active video stream.
   * @return {Promise<boolean>}
   */
  static async isPortraitModeSupported() {
    const deviceOperator = await DeviceOperator.getInstance();
    if (!deviceOperator) {
      return false;
    }
    const track = Tast.previewVideo.srcObject.getVideoTracks()[0];
    return deviceOperator.isPortraitModeSupported(track.getSettings().deviceId);
  }

  /**
   * Toggle expert mode by simulating the activation key press.
   */
  static toggleExpertMode() {
    document.body.dispatchEvent(new KeyboardEvent(
        'keydown', {ctrlKey: true, shiftKey: true, key: 'E'}));
  }

  /**
   * Sets observer of the configuration process.
   * @return {!Promise} Promise resolved/rejected after configuration finished.
   */
  static waitNextConfiguration() {
    const CAMERA_CONFIGURING = state.State.CAMERA_CONFIGURING;
    if (state.get(CAMERA_CONFIGURING)) {
      throw new Error('Already in configuring state');
    }

    return new Promise((resolve, reject) => {
      let activated = false;
      const observer = (value) => {
        if (activated === value) {
          state.removeObserver(CAMERA_CONFIGURING, observer);
          reject(new Error(
              `State ${CAMERA_CONFIGURING} assertion failed,` +
              `expecting ${!activated} got ${value}`));
          return;
        }
        if (value) {
          activated = true;
          return;
        }
        state.removeObserver(CAMERA_CONFIGURING, observer);
        resolve();
      }
      state.addObserver(CAMERA_CONFIGURING, observer);
    });
  }

  /**
   * Observes state change event for |name| state changing from |!expected| to
   * |expected|.
   * @param {string} name
   * @param {boolean} expected
   * @return {!Promise<number>} Promise resolved to the millisecond unix
   *     timestamp of when the change happen.
   */
  static async observeStateChange(name, expected) {
    const s = state.assertState(name);
    if (state.get(s) !== !expected) {
      throw new Error(`The current "${s}" state is not ${!expected}`);
    }
    return new Promise((resolve, reject) => {
      const onChange = (changed) => {
        state.removeObserver(s, onChange);
        if (changed !== expected) {
          reject(new Error(`The changed "${s}" state is not ${expected}`));
        }
        resolve(Date.now());
      };
      state.addObserver(s, onChange);
    });
  }
};
})();
