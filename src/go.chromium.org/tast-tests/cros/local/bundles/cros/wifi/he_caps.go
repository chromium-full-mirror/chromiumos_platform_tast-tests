// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"io/ioutil"
	"path/filepath"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/network/iw"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: HeCaps,
		Desc: "Verifies HE-MAC supported DUT actually supports Wifi HE protocols",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:mainline", "group:wificell", "wificell_func", "group:labqual"},
		SoftwareDeps:    []string{"wifi"},
		HardwareDeps:    hwdep.D(hwdep.Wifi80211ax()),
		Requirements:    []string{tdreq.WiFiGenSupport80211ax, tdreq.WiFiGenSupport6E, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func HeCaps(ctx context.Context, s *testing.State) {
	iwr := iw.NewLocalRunner()

	// Get the information of the WLAN device.
	res, out, err := iwr.ListPhys(ctx)
	if err != nil {
		s.Fatal("ListPhys failed: ", err)
	}
	if len(res) == 0 {
		s.Fatal("Expect at least one wireless phy; found nothing")
	}
	// Save `iw list` text to log file.
	ioutil.WriteFile(filepath.Join(s.OutDir(), "iw_list"), out, 0644)

	if !res[0].SupportHESTA {
		s.Error("Device doesn't support HE-MAC capabilities")
	}
	if !res[0].SupportHE40HE80STA {
		s.Error("Device doesn't support 5ghz HE-MAC capabilities")
	}
}
