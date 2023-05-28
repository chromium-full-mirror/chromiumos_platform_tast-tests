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

// Note: use a named class declaration and refer the class instance via the
// name here, instead of using anonymous class and referring it via "this",
// in order to make it simpler to call methods via Chrome Devtools Protocol.
window.Tast = class Tast {
  static getState(s) {
    return state.get(s);
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
