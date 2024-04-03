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
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

// A WebGL rendering workload used for stressing GPU.
const aquariumURL = "https://webglsamples.org/aquarium/aquarium.html"

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleRemoteUI,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Setting up a DUT remotely for power test with UI",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com", // CrOS platform power developers
			"zactu@google.com",                   // test author
		},
		ServiceDeps: []string{
			"tast.cros.power.DeviceSetupService",
			"tast.cros.power.RecorderService",
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ConnService",
		},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Attr:         []string{"group:power", "power_daily", "power_weekly"},
	})
}

// ExampleRemoteUI provides an example for measuring power with UI remotely. This test
// logs into the DUT, sets up the DUT for power measurement, opens a WebGL workload,
// measure the power under the load, save and upload power measurements and close
// workload at the end of the test.
func ExampleRemoteUI(ctx context.Context, s *testing.State) {
	// Connecting to the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Create service clients for the example test.
	crs := ui.NewChromeServiceClient(cl.Conn)
	cns := ui.NewConnServiceClient(cl.Conn)
	ds := ps.NewDeviceSetupServiceClient(cl.Conn)
	rs := ps.NewRecorderServiceClient(cl.Conn)

	// Fake login on the DUT through the chrome service.
	if _, err := crs.New(ctx, &ui.NewRequest{}, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to login on the DUT: ", err)
	}
	defer crs.Close(ctx, &empty.Empty{})

	// Reset chrome state at the end of the test such as closing opened tabs.
	defer crs.ResetState(ctx, &empty.Empty{})

	// Setting up a DUT remotely according to the setup request.
	if _, err = ds.Setup(ctx, &ps.DeviceSetupRequest{
		Ui:               ps.UIMode_DO_NOT_CHANGE_UI,
		ScreenBrightness: ps.ScreenMode_DEFAULT_SCREEN_BRIGHTNESS,
	}); err != nil {
		s.Fatal("Failed to setup DUT: ", err)
	}
	// Restoring the DUT to its original state once the test finishes.
	defer ds.Cleanup(ctx, &empty.Empty{})

	// Create a recorder that takes 1 sample every 5 second.
	if _, err = rs.Create(ctx, &ps.RecorderRequest{
		IntervalSec: 5,
	}); err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	// Close the recorder once the test finishes.
	defer rs.Close(ctx, &empty.Empty{})

	// Cooldown device.
	if _, err = rs.Cooldown(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to cool down device: ", err)
	}

	// Open a WebGL load in the browser.
	connInfo, err := cns.NewConn(ctx, &ui.NewConnRequest{Url: aquariumURL})
	if err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer cns.Close(ctx, &ui.CloseRequest{Id: connInfo.Id})

	// Start recording metrics.
	if _, err = rs.Start(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start recording metrics: ", err)
	}

	// Maintaining the power test environment and measuring for 30 seconds.
	// GoBigSleepLint: sleep to let the device run the workload.
	if err := testing.Sleep(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Stop recording metrics.
	rRes, err := rs.Stop(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to stop recording metrics: ", err)
	}
	perfVals := perf.NewValuesFromProto(rRes.GetPerfMetrics())

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
