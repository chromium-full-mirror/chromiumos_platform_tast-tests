// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/cellularui/

import (
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

// Tests that needs to run on all carriers except Verizon.
var hotspotTests = []string{
	"hotspot_abort_enable.go",
	"hotspot_auto_disable.go",
	"hotspot_disabled_when_no_upstream_network.go",
	"hotspot_enable_disable_in_lock_screen.go",
	"hotspot_policy.go",
}

func TestFixTestParams(t *testing.T) {
	getParams := func() string {
		return `
		{
			Name:      "",
			Val:       "",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_local"},
		},
		{
			Name:      "att",
			Val:       "att",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_att"},
		},
		{
			Name:      "tmobile",
			Val:       "tmobile",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_tmobile"},
		},
		{
			Name:      "softbank",
			Val:       "softbank",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_softbank"},
		},
		{
			Name:      "amarisoft",
			Val:       "amarisoft",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_amarisoft"},
		},
		{
			Name:      "vodafone",
			Val:       "vodafone",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_vodafone"},
		},
		{
			Name:      "rakuten",
			Val:       "rakuten",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_rakuten"},
		},
		{
			Name:      "ee",
			Val:       "ee",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_ee"},
		},
		{
			Name:      "kddi",
			Val:       "kddi",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_kddi"},
		},
		{
			Name:      "docomo",
			Val:       "docomo",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_docomo"},
		},
		{
			Name:      "fi",
			Val:       "fi",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_fi"},
		},
		{
			Name:      "bell",
			Val:       "bell",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_bell"},
		},
		{
			Name:      "roger",
			Val:       "roger",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_roger"},
		},
		{
			Name:      "telus",
			Val:       "telus",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_telus"},
		},
		{
			Name:      "cbrs",
			Val:       "cbrs",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_cbrs"},
		},
		{
			Name:      "linemo",
			Val:       "linemo",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_linemo"},
		},
		{
			Name:      "povo",
			Val:       "povo",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_povo"},
		},
		{
			Name:      "hanshin",
			Val:       "hanshin",
			// TODO(b/542029943): Re-enable once the test is stable.
			// ExtraAttr: []string{"cellular_carrier_hanshin"},
		},`
	}

	for _, filename := range hotspotTests {
		genparams.Ensure(t, filename, getParams())
	}
}
