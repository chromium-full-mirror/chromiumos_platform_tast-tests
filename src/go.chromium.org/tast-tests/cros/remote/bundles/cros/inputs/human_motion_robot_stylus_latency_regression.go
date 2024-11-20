// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/inputs"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	baseLatencyFileName  = "stylus_latency"
	gcodeLatencyFileName = baseLatencyFileName + ".nc"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HumanMotionRobotStylusLatencyRegression,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run a Stylus Drag Latency HMR Test",
		Contacts: []string{
			"chromeos-tango@google.com",
		},
		BugComponent: "b:189315", // ChromeOS > Platform > System > Input > Stylus
		Attr:         []string{"group:human_motion_robot", "human_motion_robot_latency"},
		ServiceDeps:  []string{"tast.cros.inputs.StylusService", "tast.cros.inputs.WaltService"},
		Timeout:      15 * time.Minute,
		Vars:         []string{"servo"},
		TestBedDeps:  []string{tbdep.ServoStateWorking},
	})
}

func HumanMotionRobotStylusLatencyRegression(ctx context.Context, s *testing.State) {
	touchhostHostname, touchhostPort, err := inputs.ParseHMRRuntimeVariables(s.DUT())
	if err != nil {
		s.Fatal("Failed to parse runtime variables: ", err)
	}

	servoSpec := s.RequiredVar("servo")
	dut := s.DUT()

	// Connect to the servo.
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctx)

	// Create a SSH Tunnel to connect to the TouchHost device from the remote drone.
	touchhostConnectionManager, err := inputs.CreateSSHTunnelToTouchhost(ctx, touchhostHostname, touchhostPort, dut)
	if err != nil {
		s.Fatal("Error setting up SSH tunnel to touchhost: ", err)
	}
	defer touchhostConnectionManager.TouchhostPortForwarder.Close()

	hmrInterface, err := inputs.NewHMRInterface(ctx, "127.0.0.1", touchhostConnectionManager.DronePort)
	if err != nil {
		s.Fatal("Error generating new HMR interface: ", err)
	}

	client, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer client.Close(ctx)

	s.Log("Turning on bottom USB on Servo")
	if err := pxy.Servo().SetOnOff(ctx, servo.BottomUSBKeyPwr, "on"); err != nil {
		s.Fatal("Failed to turn USB on: ", err)
	}

	// Turn off the WALT and laser once the test is done.
	defer func() {
		s.Log("Turning off bottom USB on Servo")
		if err := pxy.Servo().SetOnOff(ctx, servo.BottomUSBKeyPwr, "off"); err != nil {
			s.Fatal("Failed to turn USB off: ", err)
		}
	}()

	// Switch the bottom USB port on the Servo to pass through to the DUT so the
	// DUT can see the WALT device.
	s.Log("Switching USB to DUT")
	if err := pxy.Servo().SetString(ctx, servo.BottomUSBKeyMux, string(servo.USBMuxDUT)); err != nil {
		s.Fatal("Failed to switch USB to DUT: ", err)
	}

	// After switching the USB to the DUT, the serial port takes some time to
	// appear. Wait until it exists before starting the WALT command.
	s.Log("Waiting for WALT serial port to be visible on the DUT")
	if err := linuxssh.WaitUntilFileExists(ctx, dut.Conn(), inputs.DefaultWaltSerialPort, time.Second*30, time.Second); err != nil {
		s.Fatalf("Failed to wait for file %q to exist: %v", inputs.DefaultWaltSerialPort, err)
	}

	dutStylusService := inputspb.NewStylusServiceClient(client.Conn)
	stylusResponse, err := dutStylusService.FindPhysicalStylus(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to run FindPhysicalStylus: ", err)
	}

	serviceErrorChannel := make(chan error)
	dutWaltService := inputspb.NewWaltServiceClient(client.Conn)
	waltRequest := &inputspb.WaltRequest{TestType: "drag", InputDevicePath: stylusResponse.GetStylusPath()}

	var waltOutput string
	go func() {
		// Start measuring latency using WALT on the DUT.
		dutResponse, err := dutWaltService.StartWaltService(ctx, waltRequest)
		if err != nil {
			serviceErrorChannel <- errors.Wrap(err, "failed to run StartWaltService")
			return
		}

		waltOutput = dutResponse.GetOutput()
		s.Log("WALT output: ", waltOutput)

		serviceErrorChannel <- nil
	}()

	runJobChannel := inputs.RunAsync(ctx, func(ctx context.Context) error {
		return inputs.RunHMRJob(ctx, hmrInterface, gcodeLatencyFileName)
	})
	if err := <-runJobChannel; err != nil {
		// Ensure that the WaltService is stopped.
		if _, stopErr := dutWaltService.StopWaltService(ctx, &empty.Empty{}); stopErr == nil {
			// Only wait if the WALT service is successfully stopped.
			<-serviceErrorChannel
		}
		s.Fatal("Failed to run HMR job: ", err)
	}

	// Stop measuring latency using WALT on the DUT.
	if _, err := dutWaltService.StopWaltService(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to run StopWaltService: ", err)
	}

	if _, err := dutStylusService.CheckPhysicalStylusBatteryLevel(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to run CheckPhysicalStylusBatteryLevel: ", err)
	}

	// Wait until WALT output has been retrieved.
	serviceResponse := <-serviceErrorChannel
	if serviceResponse != nil {
		s.Fatal("Failed to collect WALT output from DUT: ", serviceResponse)
	}

	latencyResult, err := inputs.DetermineStylusLatencyVerdict(waltOutput)
	if err != nil {
		s.Fatal("Errors occurred while trying to determine verdict: ", err)
	}

	if latencyResult.Passed {
		s.Logf("Passed Test: Stylus latency (%v) was less than threshold (%v)", latencyResult.AvgLatency, inputs.StylusLatencyThreshold)
	} else if err == nil {
		// Stylus latency is irrelevant if an error is returned.
		s.Errorf("Failed Test: Stylus latency (%v) was greater than threshold (%v)", latencyResult.AvgLatency, inputs.StylusLatencyThreshold)
	}

	s.Logf("Average latency: %f", latencyResult.AvgLatency)
	s.Logf("Maximum latency: %f", latencyResult.MaxLatency)
	s.Logf("Minimum latency: %f", latencyResult.MinLatency)

	if err := inputs.SaveLatencyMetrics(latencyResult, s.OutDir()); err != nil {
		s.Error("Failed to save latency metrics: ", err)
	}
}
