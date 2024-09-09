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

type testParam struct {
	isAVL        bool
	runOnVerizon bool
}

// Map tests to whether or not they belong in AVL and need to be run on Verizon.
var standardTests = map[string]testParam{
	"hotspot_abort_enable.go":                         testParam{isAVL: false, runOnVerizon: false},
	"hotspot_auto_disable.go":                         testParam{isAVL: false, runOnVerizon: false},
	"hotspot_disabled_when_no_upstream_network.go":    testParam{isAVL: false, runOnVerizon: false},
	"hotspot_enable_disable_in_lock_screen.go":        testParam{isAVL: false, runOnVerizon: false},
	"hotspot_policy.go":                               testParam{isAVL: false, runOnVerizon: false},
	"hotspot_update_configuration.go":                 testParam{isAVL: false, runOnVerizon: false},
	"hotspot_update_configuration_when_hotspot_on.go": testParam{isAVL: false, runOnVerizon: false},
	"is_connected.go":                                 testParam{isAVL: false, runOnVerizon: true},
	"smoke.go":                                        testParam{isAVL: true, runOnVerizon: true},
	"smoke_ip_connectivity.go":                        testParam{isAVL: true, runOnVerizon: true},
}

func TestFixTestParams(t *testing.T) {
	getParams := func(param testParam) string {
		localParams := `
		{
			Name:      "",
			Val:       "",
			ExtraAttr: []string{"cellular_carrier_local"},
		},`
		if param.isAVL {
			localParams = `
		{
			Name:      "",
			Val:       "",
			ExtraAttr: []string{"cellular_carrier_local", "cellular_ota_avl"},
		},`
		}
		localParams += `
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
		},`
		if param.runOnVerizon {
			localParams += `
		{
			Name:      "verizon",
			Val:       "verizon",
			ExtraAttr: []string{"cellular_carrier_verizon"},
		},`
		}
		return localParams + `
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
		},
		{
			Name:      "cbrs",
			Val:       "cbrs",
			ExtraAttr: []string{"cellular_carrier_cbrs"},
		},
		{
			Name:      "linemo",
			Val:       "linemo",
			ExtraAttr: []string{"cellular_carrier_linemo"},
		},
		{
			Name:      "povo",
			Val:       "povo",
			ExtraAttr: []string{"cellular_carrier_povo"},
		},
		{
			Name:      "hanshin",
			Val:       "hanshin",
			ExtraAttr: []string{"cellular_carrier_hanshin"},
		},`
	}

	for filename, param := range standardTests {
		genparams.Ensure(t, filename, getParams(param))
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
