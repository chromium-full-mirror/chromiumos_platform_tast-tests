#!/bin/bash
# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

. "$(dirname "$0")/common.sh"

KDATAKEY_VERSION=$1

pushd /usr/local/tmp/faft/autest/keys

make_pair "kernel_data_key" $KERNEL_DATAKEY_ALGOID $KDATAKEY_VERSION
make_keyblock "kernel" $KERNEL_KEYBLOCK_MODE "kernel_data_key" "kernel_subkey"

popd
