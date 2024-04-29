// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/remote/camera/camerabox"
	pb "go.chromium.org/tast-tests/cros/services/cros/camerabox"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HAL3Remote,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies camera HAL3 interface function on remote DUT",
		Contacts:     []string{"chromeos-camera-eng@google.com", "beckerh@chromium.org", "xinggu@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:camerabox"},
		SoftwareDeps: []string{"arc", "arc_camera3", caps.BuiltinCamera},
		ServiceDeps:  []string{"tast.cros.camerabox.HAL3Service"},
		Data:         []string{"third_party/cts_portrait_scene.jpg"},
		Fixture:      "cameraboxFixture",
		Vars:         []string{"chart"},
		// For extra params, reference corresponding tests in:
		// src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/camera/hal3_*.go
		Params: []testing.Param{
			{
				Name:      "frame_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_FRAME, Facing: pb.Facing_FACING_BACK},
				Timeout:   15 * time.Minute,
			},
			{
				Name:      "frame_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_FRAME, Facing: pb.Facing_FACING_FRONT},
				Timeout:   15 * time.Minute,
			},

			{
				Name:      "perf_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_PERF, Facing: pb.Facing_FACING_BACK},
			},
			{
				Name:      "perf_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_PERF, Facing: pb.Facing_FACING_FRONT},
			},

			{
				Name:      "preview_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_PREVIEW, Facing: pb.Facing_FACING_BACK},
			},
			{
				Name:      "preview_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_PREVIEW, Facing: pb.Facing_FACING_FRONT},
			},

			{
				Name:      "recording_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_RECORDING, Facing: pb.Facing_FACING_BACK},
			},
			{
				Name:      "recording_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_RECORDING, Facing: pb.Facing_FACING_FRONT},
			},

			{
				Name:      "still_capture_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_STILL_CAPTURE, Facing: pb.Facing_FACING_BACK},
				Timeout:   6 * time.Minute,
			},
			{
				Name:      "still_capture_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_STILL_CAPTURE, Facing: pb.Facing_FACING_FRONT},
				Timeout:   6 * time.Minute,
			},

			{
				Name:      "still_capture_zsl_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_STILL_CAPTURE_ZSL, Facing: pb.Facing_FACING_BACK},
				Timeout:   6 * time.Minute,
			},
			{
				Name:      "still_capture_zsl_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_STILL_CAPTURE_ZSL, Facing: pb.Facing_FACING_FRONT},
				Timeout:   6 * time.Minute,
			},

			{
				Name:      "aue_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_AUE, Facing: pb.Facing_FACING_BACK},
				Timeout:   10 * time.Minute,
			},
			{
				Name:      "aue_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_AUE, Facing: pb.Facing_FACING_FRONT},
				Timeout:   10 * time.Minute,
			},

			{
				Name:      "device_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_DEVICE, Facing: pb.Facing_FACING_BACK},
				Timeout:   10 * time.Minute,
			},
			{
				Name:      "device_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_DEVICE, Facing: pb.Facing_FACING_FRONT},
				Timeout:   10 * time.Minute,
			},

			{
				Name:              "jda_back",
				ExtraAttr:         []string{"camerabox_facing_back"},
				ExtraSoftwareDeps: []string{caps.HWDecodeJPEG, caps.BuiltinUSBCamera},
				Val:               &pb.RunTestRequest{Test: pb.HAL3CameraTest_JDA, Facing: pb.Facing_FACING_BACK},
				Timeout:           10 * time.Minute,
			},
			{
				Name:              "jda_front",
				ExtraAttr:         []string{"camerabox_facing_front"},
				ExtraSoftwareDeps: []string{caps.HWDecodeJPEG, caps.BuiltinUSBCamera},
				Val:               &pb.RunTestRequest{Test: pb.HAL3CameraTest_JDA, Facing: pb.Facing_FACING_FRONT},
				Timeout:           10 * time.Minute,
			},

			{
				Name:              "jea_mipi_back",
				ExtraAttr:         []string{"camerabox_facing_back"},
				ExtraSoftwareDeps: []string{caps.HWEncodeJPEG, caps.BuiltinMIPICamera},
				Val:               &pb.RunTestRequest{Test: pb.HAL3CameraTest_JEA, Facing: pb.Facing_FACING_BACK},
				Timeout:           10 * time.Minute,
			},
			{
				Name:              "jea_mipi_front",
				ExtraAttr:         []string{"camerabox_facing_front"},
				ExtraSoftwareDeps: []string{caps.HWEncodeJPEG, caps.BuiltinMIPICamera},
				Val:               &pb.RunTestRequest{Test: pb.HAL3CameraTest_JEA, Facing: pb.Facing_FACING_FRONT},
				Timeout:           10 * time.Minute,
			},

			{
				Name:              "jea_usb_back",
				ExtraAttr:         []string{"camerabox_facing_back"},
				ExtraSoftwareDeps: []string{caps.HWEncodeJPEG, caps.BuiltinUSBCamera},
				Val:               &pb.RunTestRequest{Test: pb.HAL3CameraTest_JEA_USB, Facing: pb.Facing_FACING_BACK},
				Timeout:           10 * time.Minute,
			},
			{
				Name:              "jea_usb_front",
				ExtraAttr:         []string{"camerabox_facing_front"},
				ExtraSoftwareDeps: []string{caps.HWEncodeJPEG, caps.BuiltinUSBCamera},
				Val:               &pb.RunTestRequest{Test: pb.HAL3CameraTest_JEA_USB, Facing: pb.Facing_FACING_FRONT},
				Timeout:           10 * time.Minute,
			},

			{
				Name:      "module_back",
				ExtraAttr: []string{"camerabox_facing_back"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_MODULE, Facing: pb.Facing_FACING_BACK},
				Timeout:   10 * time.Minute,
			},
			{
				Name:      "module_front",
				ExtraAttr: []string{"camerabox_facing_front"},
				Val:       &pb.RunTestRequest{Test: pb.HAL3CameraTest_MODULE, Facing: pb.Facing_FACING_FRONT},
				Timeout:   10 * time.Minute,
			},
		},
	})
}

func HAL3Remote(ctx context.Context, s *testing.State) {
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}
	d := s.DUT()
	runTestRequest := s.Param().(*pb.RunTestRequest)
	fixt := s.FixtValue().(camerabox.FixtureData)
	// Prepare the chart for testing.
	var altHostname string
	if hostname, ok := s.Var("chart"); ok {
		altHostname = hostname
	}

	if err := fixt.PrepareChart(ctx, s.DUT(), s.OutDir(), altHostname, s.DataPath("third_party/cts_portrait_scene.jpg")); err != nil {
		s.Error("Failed to prepare chart: ", err)
	}

	// Log current test scene.
	if err := fixt.LogTestScene(ctx, d, runTestRequest.Facing, s.OutDir()); err != nil {
		s.Error("Failed to take a photo of test scene: ", err)
	}

	// Connect to the gRPC server on the DUT.
	cl, err := fixt.ConnectToDUT(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Fail to connect to the dut: ", err)
	}

	// Run remote test on DUT.
	hal3Client := pb.NewHAL3ServiceClient(cl.Conn)
	response, err := hal3Client.RunTest(ctx, runTestRequest)
	if err != nil {
		s.Fatal("Remote call RunTest() failed: ", err)
	}

	// Check test result.
	switch response.Result {
	case pb.TestResult_TEST_RESULT_PASSED:
	case pb.TestResult_TEST_RESULT_FAILED:
		s.Error("Remote test failed with error message:", response.Error)
	case pb.TestResult_TEST_RESULT_UNSET:
		s.Error("Remote test result is unset")
	}
}
