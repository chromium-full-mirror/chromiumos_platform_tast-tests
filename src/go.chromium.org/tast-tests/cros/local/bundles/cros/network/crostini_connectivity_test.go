// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/network/

// See src/go.chromium.org/tast-tests/cros/local/crostini/params.go for more documentation

import (
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/crostini"
)

func TestCrostiniConnectivityParams(t *testing.T) {
	params := crostini.MakeTestParamsFromList(t, []crostini.Param{
		{
			Name:          "dualstack",
			Timeout:       2 * time.Minute,
			IsNotMainline: true,
			UseFixture:    true,
			Val:           "guestos.ConnectivityTestParams{}",
		},
		{
			Name:          "v6only",
			Timeout:       2 * time.Minute,
			IsNotMainline: true,
			UseFixture:    true,
			Val:           "guestos.ConnectivityTestParams{V6Only: true}",
		},
	})
	genparams.Ensure(t, "crostini_connectivity.go", params)
}
