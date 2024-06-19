#!/bin/bash
# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

. "$(dirname "$0")/common.sh"

KSUBKEY_VERSION=$1

pushd /usr/local/tmp/faft/autest/keys

make_pair "kernel_subkey" $KERNEL_SUBKEY_ALGOID $KSUBKEY_VERSION

popd
