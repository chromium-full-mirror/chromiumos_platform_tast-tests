// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/

// See src/go.chromium.org/tast-tests/cros/local/crostini/params.go for more documentation

import (
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/crostini"
)

// MinimalSet is used because this test does not actually involve the container.
func TestChronyParams(t *testing.T) {
	params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
		Timeout:    2 * time.Minute,
		MinimalSet: true,
		UseFixture: true,
	}})
	genparams.Ensure(t, "chrony.go", params)
}
