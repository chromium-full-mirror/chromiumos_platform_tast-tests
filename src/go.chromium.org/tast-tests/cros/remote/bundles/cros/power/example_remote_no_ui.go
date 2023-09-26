// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	ps "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleRemoteNoUI,
		Desc:         "Setting up a DUT remotely for power test",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com", // CrOS platform power developers
			"zactu@google.com",                   // test author
		},
		ServiceDeps: []string{"tast.cros.power.DeviceSetupService"},
		Timeout:     1 * time.Minute,
		Params: []testing.Param{{
			Name: "default",
			Val: &ps.DeviceSetupRequest{
				UiAndBacklight: ps.UIAndBacklightMode_DISABLE_UI_WITH_ZERO_BACKLIGHT,
			},
		}, {
			Name: "no_wifi",
			Val: &ps.DeviceSetupRequest{
				UiAndBacklight: ps.UIAndBacklightMode_DISABLE_UI_WITH_ZERO_BACKLIGHT,
				Wifi:           ps.WifiInterfacesMode_DISABLE_WIFI_INTERFACES,
			},
		}},
	})
}

// ExampleRemoteNoUI sets up a dut for power test remotely.
func ExampleRemoteNoUI(ctx context.Context, s *testing.State) {
	// Connecting to the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Creating a device setup service.
	ds := ps.NewDeviceSetupServiceClient(cl.Conn)

	// Setting up a DUT remotely according to the setup request.
	request := s.Param().(*ps.DeviceSetupRequest)
	if _, err = ds.Setup(ctx, request); err != nil {
		s.Fatal("Failed to setup DUT: ", err)
	}
	// Restoring the DUT to its original state once the test finishes.
	defer ds.Cleanup(ctx, &empty.Empty{})

	// Maintaining the power test environment and idle for 10 seconds.
	// GoBigSleepLint: sleep to let the device idle.
	testing.Sleep(ctx, 10*time.Second)
}
