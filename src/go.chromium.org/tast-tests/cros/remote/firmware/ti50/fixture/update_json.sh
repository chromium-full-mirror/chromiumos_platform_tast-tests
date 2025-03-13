#!/bin/bash
# Copyright 2025 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Copy json file from ti50 to tast-tests, deleting any comments.
function scrub {
  sed 's+ *//.*++' < ../../../../../../../../../ti50/common/$1 \
    | sed 's+ */\*.*\*/ *++g' \
    | sed '/^[ \t]*\(\/\?\*\|$\)/d' > data/$2
}

scrub ports/haven/software/tools/cr50_haven.json cr50_h1.json
scrub ports/host_emulation/software/tools/ti50_host_emulation.json ti50_he.json
scrub ports/dauntless/software/tools/ti50_dauntless.json ti50_dt.json
scrub ports/opentitan/software/tools/ti50_opentitan.json ti50_ot.json
