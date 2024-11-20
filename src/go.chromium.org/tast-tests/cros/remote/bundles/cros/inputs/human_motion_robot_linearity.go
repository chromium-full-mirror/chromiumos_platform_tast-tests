// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	input "go.chromium.org/tast-tests/cros/remote/inputs"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	baseFileName     = "linearity"
	gcodeFileName    = baseFileName + ".nc"
	touchLogFileName = baseFileName + ".csv"
)

type serviceResponse struct {
	widthResolution  uint32
	heightResolution uint32
	err              error
}

func init() {
	testing.AddTest(&testing.Test{
		Func: HumanMotionRobotLinearity,
		Desc: "Run a linearity HMR Test",
		Contacts: []string{
			"chromeos-tango@google.com",
			"wmahon@google.com", // Test author
		},
		BugComponent: "b:189315", // ChromeOS > Platform > System > Input > Stylus
		Attr:         []string{"group:human_motion_robot", "human_motion_robot_linearity"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.inputs.StylusEvtestCaptureService"},
		Timeout:      15 * time.Minute,
	})
}

// HumanMotionRobotLinearity runs a set of stylus touch motions on a DUT, then captures
// the DUT's evtest stylus output.
func HumanMotionRobotLinearity(ctx context.Context, s *testing.State) {
	touchhostHostname, touchhostPort, err := input.ParseHMRRuntimeVariables(s.DUT())
	if err != nil {
		s.Fatal("Failed to parse runtime variables: ", err)
	}

	hostTouchLogFilePath := filepath.Join(s.OutDir(), touchLogFileName)
	hostRawTouchLogFilePath := filepath.Join(s.OutDir(), "raw_"+touchLogFileName)

	// Create a SSH Tunnel to connect to the TouchHost device from the remote drone.
	touchhostConnectionManager, err := input.CreateSSHTunnelToTouchhost(ctx, touchhostHostname, touchhostPort, s.DUT())
	if err != nil {
		s.Fatal("Error setting up SSH tunnel to touchhost: ", err)
	}
	defer touchhostConnectionManager.TouchhostPortForwarder.Close()

	hmrInterface, err := input.NewHMRInterface(ctx, "127.0.0.1", touchhostConnectionManager.DronePort)
	if err != nil {
		s.Fatal("Error generating new HMR interface: ", err)
	}

	client, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer client.Close(ctx)

	serviceChannel := make(chan serviceResponse)
	DutEvtestService := inputspb.NewStylusEvtestCaptureServiceClient(client.Conn)

	go func() {
		// Start recording evtest stylus touch data from DUT.
		dutResponse, err := DutEvtestService.StartStylusDataCapture(ctx, &empty.Empty{})
		if err != nil {
			serviceChannel <- serviceResponse{widthResolution: 0, heightResolution: 0, err: errors.Wrap(err, "failed to run StartStylusDataCapture")}
			return
		}
		// Copy file from DUT to Host machine.
		dutTouchLogFilePath := dutResponse.GetStylusLogPath()
		err = linuxssh.GetFile(ctx, s.DUT().Conn(), dutTouchLogFilePath, hostRawTouchLogFilePath, linuxssh.PreserveSymlinks)
		if err != nil {
			serviceChannel <- serviceResponse{widthResolution: 0, heightResolution: 0, err: errors.Wrap(err, "failed to copy file from DUT to Host")}
			return
		}
		serviceChannel <- serviceResponse{widthResolution: dutResponse.GetWidthResolution(), heightResolution: dutResponse.GetHeightResolution(), err: nil}
	}()

	runJobChannel := input.RunAsync(ctx, func(ctx context.Context) error {
		return input.RunHMRJob(ctx, hmrInterface, gcodeFileName)
	})
	if err := <-runJobChannel; err != nil {
		// Ensure that the evtest service is stopped.
		if _, stopErr := DutEvtestService.StopStylusDataCapture(ctx, &empty.Empty{}); stopErr == nil {
			// Only wait if the evtest service is successfully stopped.
			<-serviceChannel
		}
		s.Fatal("Failed to run HMR job: ", err)
	}

	// Stop DUT evtest stylus touch data capture.
	if _, err = DutEvtestService.StopStylusDataCapture(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to run StopStylusDataCapture: ", err)
	}
	// Wait until stylus touch data file has been copied from DUT to Host.
	serviceResponse := <-serviceChannel
	if serviceResponse.err != nil {
		s.Fatal("Failed to collect touch logs from DUT: ", serviceResponse.err)
	}

	// Delete stylus touch data file from DUT.
	if _, err = DutEvtestService.CleanUp(ctx, &empty.Empty{}); err != nil {
		s.Error("Failed to run CleanUp: ", err)
	}

	// Clean stylus touch data of common errors.
	err = input.RemoveCommonDataErrorsFromStylusLogFile(hostRawTouchLogFilePath, hostTouchLogFilePath)
	if err != nil {
		s.Error("Failed to clean raw touchlog file: ", err)
	}

	results, err := input.DetermineSingleLineVerdict(hostTouchLogFilePath, float64(serviceResponse.widthResolution), float64(serviceResponse.heightResolution))
	if err != nil {
		s.Error("Error occurred whilst generating verdict: ", err)
	}
	for _, result := range results {
		if result.Passed {
			s.Log(result.Message)
		} else {
			s.Error(result.Message)
		}
	}
}
