// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"
	"go.chromium.org/tast-tests/cros/remote/cellular/modulepower"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/hwdep"
)

const runIterations = 5

type modulePowerTestCase struct {
	connectionOptions *manager.ConfigureCallboxRequestBody
	initState         initFunction
	config            *modulepower.Config
	testTime          time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModulePower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the power consumption of the DUT is in the expected range",
		Contacts:     []string{"cros-cellular-core@google.com", "jstanko@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_cmw_callbox", "cellular_cmx_callbox", "cellular_run_isolated", "cellular_power"},
		ServiceDeps: []string{
			"tast.cros.cellular.RemoteCellularService",
			"tast.cros.power.RecorderService",
			"tast.cros.power.DeviceSetupService",
		},
		SoftwareDeps: []string{"chrome"},
		// Restrict tests to models that we have deployed intrusive measurements for.
		HardwareDeps: hwdep.D(hwdep.Model("crota", "redrix")),
		Fixture:      "callboxManagedFixture",
		Timeout:      15 * time.Minute,
		Vars:         []string{"servo"},
		Params: []testing.Param{
			{
				Name:      "low_power",
				ExtraData: []string{"cellular_power_crota_FM101.xml", "cellular_power_redrix_FM350.xml"},
				Val: modulePowerTestCase{
					connectionOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyLTE,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(
								manager.BandOption(3),
							),
						},
					},
					initState: disable(),
					testTime:  120 * time.Second,
					config: &modulepower.Config{
						// Unfortunately we can't set the measurement interval too low without
						// causing the snapshot to fail.
						MeasurementInterval: 400 * time.Millisecond,
					},
				},
			},
		},
	})
}

// initFunction is a function that puts the device in some steady state before performing measurements.
type initFunction func(context.Context, *testing.State)

func disable() initFunction {
	return func(ctx context.Context, s *testing.State) {
		tf := s.FixtValue().(*manager.TestFixture)
		if _, err := tf.RemoteCellularClient.Disable(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to disable cellular: ", err)
		}
	}
}

func ModulePower(ctx context.Context, s *testing.State) {
	tc := s.Param().(modulePowerTestCase)
	tf := s.FixtValue().(*manager.TestFixture)
	dutConn := s.DUT().Conn()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Connect to the labstation/proxy host.
	sshOptions := &ssh.Options{
		KeyDir:       s.DUT().KeyDir(),
		KeyFile:      s.DUT().KeyFile(),
		ProxyCommand: s.DUT().ProxyCommand(),
	}

	// Get the servo hostname from the servo arg.
	hostname, _ := s.Var("servo")
	if host, _, err := net.SplitHostPort(hostname); err != nil {
		s.Fatal("Failed to parse servo hostname")
	} else if host != "localhost" && host != "127.0.0.1" {
		// If host is not localhost, then we should replace port number with 22, as normally
		// the servo port is the port that servod session is running on.
		hostname = fmt.Sprintf("%s:%d", host, 22)
	}

	if err := ssh.ParseTarget(hostname, sshOptions); err != nil {
		s.Fatal("Failed to parse labstation ssh target: ", err)
	}

	proxy, err := ssh.New(ctx, sshOptions)
	if err != nil {
		s.Fatal("Failed to connect to labstation phone host over ssh: ", err)
	}

	// Prepare data path on labstation/power measurement proxy.
	model, err := reporters.New(s.DUT()).Model(ctx)
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}

	// Get Modem type to apply throughput values accordingly.
	resp, err := tf.RemoteCellularClient.QueryModemType(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get modemType: ", err)
	}
	modemType := cellularconst.ModemType(resp.ModemType)

	// Create sweetberry recorder for module power.
	configFile := s.DataPath(fmt.Sprintf("cellular_power_%s_%s.xml", model, &modemType))
	recorder, err := modulepower.NewSweetberryRecorder(ctx, proxy, configFile, tc.config)
	if err != nil {
		s.Fatal("Failed to create sweetberry recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	// Initialize remote power client for local measurements.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to DUT RPC service: ", err)
	}
	defer cl.Close(cleanupCtx)

	powerClient := power.NewRecorderServiceClient(cl.Conn)
	if _, err := powerClient.Create(ctx, &power.RecorderRequest{IntervalSec: 5}); err != nil {
		s.Fatal("Failed to set up recorder: ", err)
	}

	// Configure DUT for power settings.
	ds := power.NewDeviceSetupServiceClient(cl.Conn)
	setupRequest := &power.DeviceSetupRequest{
		Ui:               power.UIMode_DISABLE_UI,
		ScreenBrightness: power.ScreenMode_ZERO_SCREEN_BRIGHTNESS,
		Wifi:             power.WifiMode_DISABLE_WIFI,
	}
	// Setting up a DUT remotely according to the setup request.
	if _, err = ds.Setup(ctx, setupRequest); err != nil {
		s.Fatal("Failed to setup DUT: ", err)
	}
	defer ds.Cleanup(ctx, &empty.Empty{})

	// Configure the requested network on the callbox.
	if err := tf.ConnectToCallbox(ctx, dutConn, tc.connectionOptions); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	if err := recorder.Start(ctx); err != nil {
		s.Fatal("Failed to start power measurement: ", err)
	}

	testing.ContextLog(ctx, "Start recording power metrics")
	if _, err := powerClient.Start(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start recording power metrics: ", err)
	}

	s.Log("Initializing DUT to steady state")
	tc.initState(ctx, s)

	testing.ContextLog(ctx, "Cooling down device")
	if _, err := powerClient.Cooldown(ctx, &empty.Empty{}); err != nil {
		s.Log("Failed to cooldown device: ", err)
	}

	s.Log("Measuring power for ", tc.testTime)
	startTime := time.Now()
	// GoBigSleepLint: This is the length of the test to measure.
	testing.Sleep(ctx, tc.testTime)
	endTime := time.Now()

	fullResults, err := recorder.Stop(ctx)
	if err != nil {
		s.Fatal("Failed to stop power measurement: ", err)
	}

	localResults, err := powerClient.Stop(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to stop local recorder: ", err)
	}

	results, err := cp.TrimSubtestResults(startTime, endTime, fullResults)
	if err != nil {
		s.Fatal("Failed to trim results: ", err)
	}

	// Merge values after trimming since we may want the local power results over the entire test.
	fullResults.Merge(perf.NewValuesFromProto(localResults.GetPerfMetrics()))

	if err = results.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf data for crosbolt: ", err)
	}
}
