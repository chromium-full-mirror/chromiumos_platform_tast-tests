// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/action"
	reporters "go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	input "go.chromium.org/tast-tests/cros/remote/inputs"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
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
	hostTouchLogFileName := baseFileName + ".csv"

	hostTouchLogFilePath := filepath.Join(s.OutDir(), hostTouchLogFileName)
	hostRawTouchLogFilePath := filepath.Join(s.OutDir(), "raw_"+hostTouchLogFileName)

	// Checks that the GCode file is already loaded on HMR.
	fileExists, err := hmrInterface.RPC("FileExists").Args(gcodeFileName).CallForBool(ctx)
	if err != nil {
		s.Fatal("Failed to call FileExists: ", err)
	}
	if !fileExists {
		s.Fatalf("Gcode file (%s) not found on HMR", gcodeFileName)
	}

	serviceErrorChannel := make(chan error)
	dutEvtestService := inputspb.NewStylusEvtestCaptureServiceClient(client.Conn)

	go func() {
		// Start recording evtest stylus touch data from DUT.
		dutResponse, err := dutEvtestService.StartStylusDataCapture(ctx, &empty.Empty{})
		if err != nil {
			serviceErrorChannel <- errors.Wrap(err, "failed to run StartStylusDataCapture")
			return
		}
		// Copy file from DUT to Host machine.
		dutTouchLogFilePath := dutResponse.GetStylusLogPath()
		if err := linuxssh.GetFile(ctx, s.DUT().Conn(), dutTouchLogFilePath, hostRawTouchLogFilePath, linuxssh.PreserveSymlinks); err != nil {
			serviceErrorChannel <- errors.Wrap(err, "failed to copy file from DUT to Host")
			return
		}
		serviceErrorChannel <- nil
	}()

	// Begins executing HMR motions on DUT.
	if err := hmrInterface.RPC("StartJob").Args(gcodeFileName).Call(ctx); err != nil {
		s.Fatal("Failed to start job: ", err)
	}

	// Polls TouchHost for progress of HMR job. Blocks until the HMR job is complete.
	prevProgress := -1.0
	if err := action.Retry(100, func(ctx context.Context) error {
		progress, err := hmrInterface.RPC("GetProgress").Args().CallForFloat64(ctx)
		if err != nil {
			s.Fatal("Failed to poll progress: ", err)
			return err
		}
		// progress resets to 0 when job is complete.
		if prevProgress <= progress {
			prevProgress = progress
			return errors.Errorf("HMR job is still in progress (%f%%)", progress*100)
		}
		return nil
	}, 3*time.Second)(ctx); err != nil {
		s.Fatal("HMR Job did not complete in time")
		hmrInterface.RPC("StopJob").Call(ctx)
	}

	// Stop DUT evtest stylus touch data capture.
	if _, err := dutEvtestService.StopStylusDataCapture(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to run StopStylusDataCapture: ", err)
	}
	// Wait until stylus touch data file has been copied from DUT to Host.
	serviceResponse := <-serviceErrorChannel
	if serviceResponse != nil {
		s.Fatal("Failed to collect touch logs from DUT: ", serviceResponse)
	}

	// Delete stylus touch data file from DUT.
	if _, err := dutEvtestService.CleanUp(ctx, &empty.Empty{}); err != nil {
		s.Error("Failed to run CleanUp: ", err)
	}

	// Clean stylus touch data of common errors.
	if err := input.RemoveCommonDataErrorsFromStylusLogFile(hostRawTouchLogFilePath, hostTouchLogFilePath); err != nil {
		s.Error("Failed to clean raw touchlog file: ", err)
	}

	referenceFilePath := s.DataPath(referenceFileData.filename + ".csv")
	fullImageResult, err := input.DetermineFullImageVerdict(referenceFilePath, hostTouchLogFilePath, calibrationData, dutScreenWidthInMM, dutScreenHeightInMM)
	if err != nil {
		s.Error("Errors occurred while trying to determine verdict: ", err)
	}

	if fullImageResult != nil {
		if fullImageResult.Passed {
			s.Logf("Passed Test: Full Image MSE (%v) was less than threshold (%v)", fullImageResult.TotalMSE, input.FullImageMSEThreshold)
		} else if err == nil {
			// Full Image MSE is irrelevant if an error is returned.
			s.Errorf("Failed Test: Full Image MSE (%v) was greater than threshold (%v)", fullImageResult.TotalMSE, input.FullImageMSEThreshold)
		}
		s.Logf("Epsilon used: %vmm", fullImageResult.Epsilon)
		s.Log("Successfully analysed paths info:")
		for _, pathResult := range fullImageResult.PathResults {
			s.Logf("Path MSE: %v, Successful fit rate: %v, Reference path length: %v, Result path length: %v, Number of calculations: %v, Bubble radius: %vmm",
				pathResult.PathMSE,
				pathResult.SuccessfulFitRate,
				pathResult.ReferencePathLength,
				pathResult.ResultPathLength,
				pathResult.NumberOfCalculations,
				pathResult.BubbleRadius)
		}
	}
}
