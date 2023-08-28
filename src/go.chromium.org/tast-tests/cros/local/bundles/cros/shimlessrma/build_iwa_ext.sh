#!/bin/bash
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Note: If you modify the fixture/data/iwa or fixture/data/extension, you have
# to rebuild them by running build_iwa_ext.sh.

# Build IWA.
cd fixture/data/iwa/ || exit
npm install
npm run build

# Build extension.
cd ../extension || exit
npm install

# Pack extension.
/usr/bin/google-chrome --pack-extension=./build \
                       --pack-extension-key=private_key.pem
mv build.crx diag.crx
