// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/cellular/

import (
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/crostini"
)

// Map tests to whether or not they belong in AVL.
var standardTests = map[string]bool{
	"is_connected.go":          false,
	"smoke.go":                 true,
	"smoke_ip_connectivity.go": true,
}

func TestFixTestParams(t *testing.T) {
	getParams := func(isAVL bool) string {
		localParams := `
		{
			Name:      "",
			Val:       "",
			ExtraAttr: []string{"cellular_carrier_local"},
		},`
		if isAVL {
			localParams = `
		{
			Name:      "",
			Val:       "",
			ExtraAttr: []string{"cellular_carrier_local", "cellular_ota_avl"},
		},`
		}
		return localParams + `
		{
			Name:      "att",
			Val:       "att",
			ExtraAttr: []string{"cellular_carrier_att"},
		},
		{
			Name:      "tmobile",
			Val:       "tmobile",
			ExtraAttr: []string{"cellular_carrier_tmobile"},
		},
		{
			Name:      "softbank",
			Val:       "softbank",
			ExtraAttr: []string{"cellular_carrier_softbank"},
		},
		{
			Name:      "amarisoft",
			Val:       "amarisoft",
			ExtraAttr: []string{"cellular_carrier_amarisoft"},
		},
		{
			Name:      "vodafone",
			Val:       "vodafone",
			ExtraAttr: []string{"cellular_carrier_vodafone"},
		},
		{
			Name:      "rakuten",
			Val:       "rakuten",
			ExtraAttr: []string{"cellular_carrier_rakuten"},
		},
		{
			Name:      "ee",
			Val:       "ee",
			ExtraAttr: []string{"cellular_carrier_ee"},
		},
		{
			Name:      "kddi",
			Val:       "kddi",
			ExtraAttr: []string{"cellular_carrier_kddi"},
		},
		{
			Name:      "docomo",
			Val:       "docomo",
			ExtraAttr: []string{"cellular_carrier_docomo"},
		},
		{
			Name:      "fi",
			Val:       "fi",
			ExtraAttr: []string{"cellular_carrier_fi"},
		},
		{
			Name:      "verizon",
			Val:       "verizon",
			ExtraAttr: []string{"cellular_carrier_verizon"},
		},
		{
			Name:      "bell",
			Val:       "bell",
			ExtraAttr: []string{"cellular_carrier_bell"},
		},
		{
			Name:      "roger",
			Val:       "roger",
			ExtraAttr: []string{"cellular_carrier_roger"},
		},
		{
			Name:      "telus",
			Val:       "telus",
			ExtraAttr: []string{"cellular_carrier_telus"},
		},`
	}

	for filename, isAVL := range standardTests {
		genparams.Ensure(t, filename, getParams(isAVL))
	}
}

var crostiniTests = map[string]time.Duration{
	"crostini_network_connectivity.go": 10 * time.Minute,
}

func TestFixCrostiniTestParams(t *testing.T) {
	for filename, duration := range crostiniTests {
		params := crostini.MakeTestParamsFromList(t, []crostini.Param{{
			Timeout:       duration,
			IsNotMainline: true,
			UseFixture:    true,
		}})
		genparams.Ensure(t, filename, params)
	}
}
