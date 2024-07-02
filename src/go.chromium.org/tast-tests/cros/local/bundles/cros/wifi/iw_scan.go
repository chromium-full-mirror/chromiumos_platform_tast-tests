// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/network/ip"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/wifi"
	"go.chromium.org/tast-tests/cros/local/wifi/iw"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: IWScan,
		Desc: "Verifies `iw` Timed Scan executes and is parsed properly",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:mainline", "group:wificell", "wificell_func"},
		SoftwareDeps:    []string{"wifi"},
		HardwareDeps:    hwdep.D(hwdep.WifiNotMarvell()),
		Fixture:         "wiphyEnabled",
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func IWScan(ctx context.Context, s *testing.State) {
	manager := s.FixtValue().(*wifi.WiphyEnabledFixtureData).ShillManager

	iface, err := shill.WifiInterface(ctx, manager, 5*time.Second)
	if err != nil {
		s.Fatal("Could not get a WiFi interface: ", err)
	}
	s.Log("WiFi interface: ", iface)

	// In order to guarantee reliable execution of IWScan, we need to make sure
	// shill doesn't interfere with the scan. We will disable shill's control
	// on the wireless device while still maintaining Ethernet connectivity.
	if err := manager.DisableTechnology(ctx, shill.TechnologyWifi); err != nil {
		s.Fatal("Could not disable WiFi from shill: ", err)
	}

	defer func() {
		// Allow shill to take control of wireless device.
		if err := manager.EnableTechnology(ctx, shill.TechnologyWifi); err != nil {
			s.Error("Could not enable WiFi from shill: ", err)
		}
	}()

	// Bring up wireless device after it's released from shill.
	ipr := ip.NewLocalRunner()
	if err := ipr.SetLinkUp(ctx, iface); err != nil {
		s.Fatalf("Could not bring up %s after shill released WiFi management", iface)
	}

	// Conduct scan
	iwr := iw.NewLocalRunner()
	if _, err = iwr.TimedScan(ctx, iface, nil, nil); err != nil {
		s.Fatal("TimedScan failed: ", err)
	}
}
