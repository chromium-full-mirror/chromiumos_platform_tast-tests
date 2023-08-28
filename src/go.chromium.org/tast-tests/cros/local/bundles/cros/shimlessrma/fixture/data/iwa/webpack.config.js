// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/* webpack.config.js */
const path = require('path');
const WebBundlePlugin = require('webbundle-webpack-plugin');
const {
  NodeCryptoSigningStrategy,
  parsePemKey,
  readPassphrase,
  WebBundleId,
} = require('wbn-sign');
const { readFileSync } = require('fs');


module.exports = async () => {
  const key = parsePemKey(readFileSync('ed25519key.pem'));
  return {
    entry: './src/index.js',
    output: {
      path: path.resolve(__dirname, 'dist'),
      filename: 'app.js'
    },
    plugins: [
      new WebBundlePlugin({
        baseURL: new WebBundleId(key).serializeWithIsolatedWebAppOrigin(),
        static: { dir: path.resolve(__dirname, 'src') },
        output: 'diag.swbn',
        integrityBlockSign: {
          strategy: new NodeCryptoSigningStrategy(key),
        },
      }),
    ],
  };
};
