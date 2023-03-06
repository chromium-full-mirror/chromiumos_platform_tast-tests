// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/trunk/src/platform/tast/tools/go.sh test -count=1 chromiumos/tast/local/bundles/cros/cellular/

import (
	"testing"
	"time"

	"chromiumos/tast/common/genparams"
	"chromiumos/tast/local/crostini"
)

var standardTests = []string{
	"autoconnect.go",
	"identifiers.go",
	"is_connected.go",
	"shill_enable_disable.go",
	"smoke.go",
	"smoke_ip_connectivity.go",
}

func TestFixTestParams(t *testing.T) {
	params := `
		{
			ExtraAttr: []string{"cellular_carrier_local"},
		},
		{
			Name:      "att",
			ExtraAttr: []string{"cellular_carrier_att"},
		},
		{
			Name:      "tmobile",
			ExtraAttr: []string{"cellular_carrier_tmobile"},
		},
		{
			Name:      "softbank",
			ExtraAttr: []string{"cellular_carrier_softbank"},
		},
		{
			Name:      "amarisoft",
			ExtraAttr: []string{"cellular_carrier_amarisoft"},
		},
		{
			Name:      "vodafone",
			ExtraAttr: []string{"cellular_carrier_vodafone"},
		},
		{
			Name:      "rakuten",
			ExtraAttr: []string{"cellular_carrier_rakuten"},
		},
		{
			Name:      "ee",
			ExtraAttr: []string{"cellular_carrier_ee"},
		},
		{
			Name:      "kddi",
			ExtraAttr: []string{"cellular_carrier_kddi"},
		},
		{
			Name:      "docomo",
			ExtraAttr: []string{"cellular_carrier_docomo"},
		},
		{
			Name:      "fi",
			ExtraAttr: []string{"cellular_carrier_fi"},
		},
		{
			Name:      "verizon",
			ExtraAttr: []string{"cellular_carrier_verizon"},
		},`
	for _, filename := range standardTests {
		genparams.Ensure(t, filename, params)
	}
}

var crostiniTests = map[string]time.Duration{
	"crostini_cellular_network_connectivity.go": 10 * time.Minute,
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
