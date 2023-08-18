// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: V4L2Compliance,
		Desc: "Runs V4L2Compliance in all the Capture Devices",
		Contacts: []string{
			"chromeos-camera-kernel@google.com",
			"chromeos-camera-eng@google.com",
			"ribalda@chromium.org",
		},
		BugComponent: "b:1093480",
		Attr:         []string{"group:mainline", "group:camera-usb-qual", "group:cq-medium"},
		// TODO(b/173778998) Jinlon privacy switch is not compliance: EBUSY during streamoff.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("jinlon")),
		SoftwareDeps: []string{"uvc_compliant"},
	})
}

func startCrosCameraService(ctx context.Context) error {
	testing.ContextLog(ctx, "Starting cros-camera")
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		return errors.Wrap(err, "failed to start cros-camera")
	}
	return nil
}

func stopCrosCameraService(ctx context.Context) error {
	testing.ContextLog(ctx, "Stopping cros-camera")
	if err := upstart.StopJob(ctx, "cros-camera"); err != nil {
		return errors.Wrap(err, "failed to stop cros-camera")
	}

	if err := upstart.WaitForJobStatus(ctx, "cros-camera", upstartcommon.StopGoal,
		upstartcommon.WaitingState, upstart.RejectWrongGoal, ctxutil.MaxTimeout); err != nil {
		startCrosCameraService(ctx)
		return errors.Wrap(err, "the cros-camera service did not stop before calling runCrosCameraTest")
	}

	return nil
}

func V4L2Compliance(ctx context.Context, s *testing.State) {
	badCameras := map[string]string{
		"13d3:5519": "b/258798506",
		"04f2:b719": "b/272738845",
		"0c45:636e": "b/281539980",
		"0c45:6a14": "b/288647798",
		"0c45:6a16": "b/287943727",
		"0408:4041": "b/262499795",
	}

	// Use a shorter context to save time for clean.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	err := stopCrosCameraService(ctx)
	if err != nil {
		s.Fatal("Error stopping cros-camera service: ", err)
	}
	defer startCrosCameraService(ctx)

	captureDevices, err := testutil.CaptureDevicesFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to list Capture Devices: ", err)
	}

	for _, videodev := range captureDevices {

		usbCameraVersion, err := testutil.GetUsbCameraVersion(ctx, videodev)
		if err == nil {
			vidPid := usbCameraVersion.IDVendor + ":" + usbCameraVersion.IDProduct
			bugNumber, found := badCameras[vidPid]
			if found {
				testing.ContextLog(ctx, "Device "+vidPid+" ignored due to: "+bugNumber)
				continue
			}
		}

		cmd := testexec.CommandContext(ctx, "v4l2-compliance", "-v", "-d", videodev)
		out, err := cmd.Output(testexec.DumpLogOnError)

		if err == nil {
			continue
		}

		// Log full output on error.
		result := string(out)
		s.Log(result)

		if cmd.ProcessState.ExitCode() != 1 {
			s.Fatalf("v4l2-compliance failed: %s: %v", videodev, err)
		}

		// Remove last end of line if present.
		result = strings.TrimSuffix(result, "\n")
		// Get last line.
		lastline := result[strings.LastIndex(result, "\n"):]
		s.Errorf("v4l2-compliance failed: %s: %s", videodev, lastline)
	}
}
