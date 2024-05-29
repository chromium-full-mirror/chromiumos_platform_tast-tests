// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	reporters "go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	input "go.chromium.org/tast-tests/cros/remote/inputs"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	simpleReferenceFileName = "human_motion_robot_full_image_simple"
	calibrationFileName     = "calibration_file.json"
)

// referenceFileData contains data associated with the input csv file used to generate the motions for a particular test.
type referenceFileData struct {
	filename string
	date     string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HumanMotionRobotFullImage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run a full image HMR Test",
		Contacts: []string{
			"chromeos-tango@google.com",
			"wmahon@google.com", // Test author
		},
		BugComponent: "b:189315", // ChromeOS > Platform > System > Input > Stylus
		Attr:         []string{"group:human_motion_robot"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.inputs.StylusEvtestCaptureService"},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			Name:      "simple",
			Val:       referenceFileData{filename: simpleReferenceFileName, date: "20240523"},
			ExtraData: []string{simpleReferenceFileName + ".csv"},
		}},
	})
}

func HumanMotionRobotFullImage(ctx context.Context, s *testing.State) {
	referenceFileData := s.Param().(referenceFileData)

	touchhostHostname, touchhostPort, err := input.ParseHMRRuntimeVariables(s.DUT())
	if err != nil {
		s.Fatal("Failed to parse runtime variables: ", err)
	}

	// Create a SSH Tunnel to connect to the TouchHost device from the remote drone.
	touchhostConnectionManager, err := input.CreateSSHTunnelToTouchhost(ctx, touchhostHostname, touchhostPort, s.DUT())
	if err != nil {
		s.Fatal("Error setting up SSH tunnel to touchhost: ", err)
	}
	defer touchhostConnectionManager.TouchhostPortForwarder.Close()

	// Create DUT client to run services on DUT.
	client, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer client.Close(ctx)

	hmrInterface, err := input.NewHMRInterface(ctx, "127.0.0.1", 9992)
	if err != nil {
		s.Fatal("Error generating new HMR interface: ", err)
	}

	// A calibration file is stored locally on the HMR.
	// A unique calibration file is generated for each HMR setup by a calibration routine.
	// It is used by the full image analyzer to correct data errors unique to this HMR setup.
	calibrationText, err := hmrInterface.RPC("GetFileContents").Args(calibrationFileName).CallForString(ctx)
	if err != nil {
		s.Fatal("Failed to query calibration file from touchhost: ", err)
	}
	var calibrationData input.CalibrationData
	err = json.Unmarshal([]byte(calibrationText), &calibrationData)
	if err != nil {
		s.Fatal("Failed to decode calibration file json string: ", err)
	}

	// Device model name is used for lookup of screen's physical dimensions.
	reporter := reporters.New(s.DUT())
	modelName, err := reporter.Model(ctx)
	if err != nil {
		s.Fatal("Could not obtain device model name: ", err)
	}

	dutScreenHeightInMM, dutScreenWidthInMM, err := input.GetScreenDimensions(modelName)
	if err != nil {
		s.Fatal("Could not obtain device's screen dimensions: ", err)
	}

	screenSize, err := input.GetDiagonalScreenSize(dutScreenHeightInMM, dutScreenWidthInMM)
	if err != nil {
		s.Fatal("Failed to get DUT screen diagnonal screen size: ", err)
	}
	aspectRatio, err := input.GetScreenAspectRatio(dutScreenHeightInMM, dutScreenWidthInMM)
	if err != nil {
		s.Fatal("Failed to get DUT screen resolution: ", err)
	}

	// Full image gcode file names are of the format {reference filename}-{diagonal screen size}-{screen aspect ratio}-{date input file was created}.
	// Example: human_motion_robot_full_image_simple-13.3-16_9-20240523.nc
	baseFileName := referenceFileData.filename + "-" + strconv.FormatFloat(screenSize, 'f', -1, 64) + "-" + aspectRatio + "-" + referenceFileData.date
	gcodeFileName := baseFileName + ".nc"

	// Checks that the GCode file is already loaded on HMR.
	fileExists, err := hmrInterface.RPC("FileExists").Args(gcodeFileName).CallForBool(ctx)
	if err != nil {
		s.Fatal("Failed to call FileExists: ", err)
	}
	if !fileExists {
		s.Fatalf("Gcode file (%s) not found on HMR", gcodeFileName)
	}

	// TODO(b/343548313): Run HMR job and collect touch events.
	// TODO(b/343548793): Full image analysis.
}
