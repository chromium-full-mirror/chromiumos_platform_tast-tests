// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
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
		ServiceDeps: []string{
			"tast.cros.power.DeviceSetupService",
			"tast.cros.power.RecorderService",
		},
		Timeout: 2 * time.Minute,
		Params: []testing.Param{{
			Name: "default",
			Val: &ps.DeviceSetupRequest{
				Ui:               ps.UIMode_DISABLE_UI,
				ScreenBrightness: ps.ScreenMode_ZERO_SCREEN_BRIGHTNESS,
			},
		}, {
			Name: "no_wifi",
			Val: &ps.DeviceSetupRequest{
				Ui:               ps.UIMode_DISABLE_UI,
				ScreenBrightness: ps.ScreenMode_ZERO_SCREEN_BRIGHTNESS,
				Wifi:             ps.WifiMode_DISABLE_WIFI,
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

	// Create a device setup service.
	ds := ps.NewDeviceSetupServiceClient(cl.Conn)

	// Setting up a DUT remotely according to the setup request.
	dReq := s.Param().(*ps.DeviceSetupRequest)
	if _, err = ds.Setup(ctx, dReq); err != nil {
		s.Fatal("Failed to setup DUT: ", err)
	}
	// Restoring the DUT to its original state once the test finishes.
	defer ds.Cleanup(ctx, &empty.Empty{})

	// Create a recorder service.
	rs := ps.NewRecorderServiceClient(cl.Conn)

	// Create a recorder that takes 1 sample every 5 second.
	rReq := &ps.RecorderRequest{
		IntervalSec: 5,
	}
	if _, err = rs.Create(ctx, rReq); err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	// Close the recorder once the test finishes.
	defer rs.Close(ctx, &empty.Empty{})

	// Cooldown device.
	if _, err = rs.Cooldown(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to cool down device: ", err)
	}

	// Start recording metrics.
	if _, err = rs.Start(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start recording metrics: ", err)
	}

	// Maintaining the power test environment and idle for 30 seconds.
	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Stop recording metrics.
	rRes, err := rs.Stop(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to stop recording metrics: ", err)
	}
	perfVals := perf.NewValuesFromProto(rRes.GetPerfMetrics())

	// TODO(b/317461515): set checkpoints on the remote side, and interact with
	// data from the local side.
	// Save recorded metrics in remote and upload to dashboard.
	if _, err := cp.CreateSaveUploadPowerLog(
		ctx,
		s.OutDir(),
		s.TestName(),
		"",
		perfVals,
		nil,
		rRes.GetDeviceInfo(),
		rRes.GetOneTimeMetrics(),
	); err != nil {
		s.Fatal("Failed to save and upload power log: ", err)
	}

	// Optionally insert custom metrics here. You must not insert
	// them before uploading to dashboard or upload may fail.
	perfVals.Set(perf.Metric{
		Name: "custom_metric",
		Unit: "unit",
	}, 10.0)

	// Save recorded metrics for crosbolt containing custom metrics
	// if any.
	if err := perfVals.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf data for crosbolt: ", err)
	}
}
