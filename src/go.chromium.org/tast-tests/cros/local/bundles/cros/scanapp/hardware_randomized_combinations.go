// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package scanapp

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/scanapp/scanning"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HardwareRandomizedCombinations,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the Scan app can be used on real hardware with randomized combinations",
		Contacts: []string{
			"cros-peripherals@google.com",
			"project-bolton@google.com",
		},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:paper-io",
			"paper-io_scanning",
		},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{
			// Format for test cases is as follows:
			// Name: manufacturer_model
			// Val: scanner descriptor file
			// ExtraData: scanner descriptor file
			{
				//MFP in lab
				Name:      "brother_dcp_l2550dw",
				Val:       "brother_dcp_l2550dw_descriptor.json",
				ExtraData: []string{"brother_dcp_l2550dw_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
			}, {
				//MFP in lab
				Name:      "hp_laserjet_pro_m478",
				Val:       "hp_laserjet_pro_m478_descriptor.json",
				ExtraData: []string{"hp_laserjet_pro_m478_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
			}, {
				//MFP in lab
				Name:      "sharp_mx_b467f",
				Val:       "sharp_mx_b467f_descriptor.json",
				ExtraData: []string{"sharp_mx_b467f_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
			}, {
				//MFP in lab
				Name:      "hp_laserjet_mfp_m234dw",
				Val:       "hp_laserjet_mfp_m234dw_descriptor.json",
				ExtraData: []string{"hp_laserjet_mfp_m234dw_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
			}, {
				//MFP in lab
				Name:      "lexmark_mc3426adw",
				Val:       "lexmark_mc3426adw_descriptor.json",
				ExtraData: []string{"lexmark_mc3426adw_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
			}, {
				//MFP in lab
				Name:              "usb_canon_tr4700_series",
				Val:               "usb_canon_tr4700_series_descriptor.json",
				ExtraData:         []string{"usb_canon_tr4700_series_descriptor.json"},
				ExtraAttr:         []string{"paper-io_mfp_printscan"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model("treeya")),
			}, {
				//MFP in lab
				Name:              "usb_hp_deskjet_2700_series",
				Val:               "usb_hp_deskjet_2700_series_descriptor.json",
				ExtraData:         []string{"usb_hp_deskjet_2700_series_descriptor.json"},
				ExtraAttr:         []string{"paper-io_mfp_printscan"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model("lazor")),
			},
		},
	})
}

func HardwareRandomizedCombinations(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	fileContents, err := ioutil.ReadFile(s.DataPath(s.Param().(string)))
	if err != nil {
		s.Fatal("Unable to read scanner descriptor file: ", err)
	}

	var scanner scanning.ScannerDescriptor
	err = json.Unmarshal(fileContents, &scanner)
	if err != nil {
		s.Fatal("Unable to unmarshal scanner descriptor file: ", err)
	}

	scanning.RunHardwareTests(ctx, s, cr, scanner, scanning.HardwareTestRunRandomizedCombinations)
}
