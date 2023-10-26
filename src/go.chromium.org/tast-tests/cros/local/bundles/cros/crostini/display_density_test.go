// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/

// See src/go.chromium.org/tast-tests/cros/local/crostini/params.go for more documentation

import (
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/crostini"
)

func TestDisplayDensityParams(t *testing.T) {
	params := crostini.MakeTestParamsFromList(t, []crostini.Param{
		{
			Name:            "wayland",
			Val:             "guestos.WaylandDemoConfig()",
			UseFixture:      true,
			LowPerfEligible: true,
		},
		{
			Name:            "x11",
			Val:             "guestos.X11DemoConfig()",
			UseFixture:      true,
			LowPerfEligible: true,
		}})
	genparams.Ensure(t, "display_density.go", params)
}
