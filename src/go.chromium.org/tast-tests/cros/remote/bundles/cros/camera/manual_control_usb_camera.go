// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/remote/camera/camerabox"
	"go.chromium.org/tast-tests/cros/services/cros/camera"
	pb "go.chromium.org/tast-tests/cros/services/cros/camerabox"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	saveImageKey                            = "camera.ManualControlUSBCamera.SaveImage"
	chartHostnameKey                        = "chart"
	chartImagePath                          = "third_party/cts_portrait_scene.jpg"
	cameraUserControlValidateScriptName     = "camera_user_control_validate.py"
	manualControlImageConfigProtoPythonName = "manual_control_image_config_pb2.py"
)

type manualControlUSBCameraParams struct {
	facing pb.Facing
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManualControlUSBCamera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the functionality of manual controlling usb cameras",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:camerabox"},
		SoftwareDeps: []string{caps.BuiltinUSBCamera, "chrome"},
		ServiceDeps:  []string{"tast.cros.camera.ManualControlUSBCameraService"},
		Data:         []string{chartImagePath, cameraUserControlValidateScriptName, manualControlImageConfigProtoPythonName},
		Fixture:      "cameraboxFixture",
		Vars:         []string{saveImageKey, chartHostnameKey},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       manualControlUSBCameraParams{facing: pb.Facing_FACING_BACK},
			},
			{
				Name:      "front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       manualControlUSBCameraParams{facing: pb.Facing_FACING_FRONT},
			},
		},
	})

}

func ManualControlUSBCamera(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	facing := s.Param().(manualControlUSBCameraParams).facing
	cameraboxFixtureData := s.FixtValue().(camerabox.FixtureData)

	// Prepare the chart for testing.
	var altHostname string
	if hostname, ok := s.Var(chartHostnameKey); ok {
		altHostname = hostname
	}

	if err := cameraboxFixtureData.PrepareChart(ctx, dut, s.OutDir(), altHostname, s.DataPath(chartImagePath)); err != nil {
		s.Fatal("Failed to prepare chart: ", err)
	}

	// Log current test scene.
	if err := cameraboxFixtureData.LogTestScene(ctx, dut, facing, s.OutDir()); err != nil {
		s.Fatal("Failed to take a photo of test scene: ", err)
	}

	// Prepare data on DUT.
	tempdir, err := dut.Conn().CommandContext(ctx, "mktemp", "-d", "/tmp/camerabox_align_XXXXXX").Output()
	tempdirPath := strings.TrimSpace(string(tempdir))
	cameraUserControlValidateScriptPath := filepath.Join(tempdirPath, cameraUserControlValidateScriptName)
	manualControlImageConfigProtoPythonPath := filepath.Join(tempdirPath, manualControlImageConfigProtoPythonName)
	defer dut.Conn().CommandContext(ctx, "rm", "-r", tempdirPath).Output()
	if _, err := linuxssh.PutFiles(
		ctx, dut.Conn(), map[string]string{
			s.DataPath(cameraUserControlValidateScriptName):     cameraUserControlValidateScriptPath,
			s.DataPath(manualControlImageConfigProtoPythonName): manualControlImageConfigProtoPythonPath,
		}, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote temp dir %s: %s", tempdirPath, err)
	}

	// Connect to the gRPC server on the DUT.
	client, err := cameraboxFixtureData.ConnectToDUT(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Fail to connect to the dut: ", err)
	}

	// Run remote test on DUT.
	manualControlClient := camera.NewManualControlUSBCameraServiceClient(client.Conn)
	saveImage := false
	if saveImageString, hasSaveImage := s.Var(saveImageKey); hasSaveImage {
		saveImage, err = strconv.ParseBool(saveImageString)
		if err != nil {
			s.Fatalf("Failed to parse %s from a string to a bool: %s", saveImageString, err)
		}
	}

	var manualControlFacing camera.Facing
	if facing == pb.Facing_FACING_BACK {
		manualControlFacing = camera.Facing_FACING_BACK
	} else if facing == pb.Facing_FACING_FRONT {
		manualControlFacing = camera.Facing_FACING_FRONT
	} else {
		manualControlFacing = camera.Facing_FACING_UNSET
	}

	if _, err = manualControlClient.ValidateControl(ctx, &camera.ValidateControlRequest{
		SaveImage:                           saveImage,
		Facing:                              manualControlFacing,
		CameraUserControlValidateScriptPath: cameraUserControlValidateScriptPath,
	}); err != nil {
		s.Fatal("Remote call RunTest() failed: ", err)
	}
}
