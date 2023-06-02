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

// Note: use a named class declaration and refer the class instance via the
// name here, instead of using anonymous class and referring it via "this",
// in order to make it simpler to call methods via Chrome Devtools Protocol.
window.Tast = class Tast {
};
})();
