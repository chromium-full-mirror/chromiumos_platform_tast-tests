// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           TestConnectToBTPeers,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that a remote test can connect to btpeers and call a chameleond method",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jaredbennett@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr: []string{
			"group:bluetooth",
			"bluetooth_btpeers_2",
		},
		SoftwareDeps: []string{"chrome"},
		TestBedDeps:  []string{tbdep.WorkingBluetoothPeers(2)},
		Timeout:      time.Second * 15,
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeUIDisabledWith2BTPeersFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:              "floss_enabled",
				Fixture:           "chromeUIDisabledWith2BTPeersFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// TestConnectToBTPeers tests that a remote test can connect to btpeers and call
// a chameleond method.
func TestConnectToBTPeers(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	if _, err := fv.BTPeers[0].ChameleondClient().GetMacAddress(ctx); err != nil {
		s.Fatal("Failed to call chameleond method 'GetMacAddress' on btpeer1: ", err)
	}
	if err := fv.BTPeers[1].ChameleondClient().BluetoothAudioDevice().Reboot(ctx); err != nil {
		s.Fatal("Failed to call chameleond method 'Reboot' on btpeer2.BluetoothAudioDevice: ", err)
	}
}
