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
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that a remote test can connect to btpeers and call a chameleond method",
		Contacts: []string{
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
		},
		BugComponent:    "b:976419", // ChromeOS > EngProd > Platform > Connectivity
		Attr:            []string{"group:bluetooth", "group:release-health", "release-health_bt"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(2)},
		SoftwareDeps:    []string{"chrome"},
		Timeout:         time.Second * 15,
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeUIDisabledWith2BTPeersFlossDisabled",
				ExtraAttr: []string{"bluetooth_sa"},
			},
			{
				Name:      "floss_enabled",
				Fixture:   "chromeUIDisabledWith2BTPeersFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss"},
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
